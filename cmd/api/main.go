// Lambda (API Handler)
//
// 作用：作为 API Gateway 的后端处理器，启动 Step Functions 执行并同步等待完成后返回。
// 链路：Client -> API Gateway -> ApiFunction -> Step Functions -> Dispatcher -> SQS -> Worker -> (callback) -> Step Functions -> ApiFunction 返回
//
// 环境变量：STATE_MACHINE_ARN（Step Functions State Machine ARN）
// 对应 SAM 资源：template.yaml 中的 ApiFunction
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
)

type apiRequest struct {
	RunID            string `json:"runId,omitempty"`
	DelaySeconds     int    `json:"delaySeconds,omitempty"`
	MessageBodyBytes int    `json:"messageBodyBytes,omitempty"`
	// 可选：客户端控制最大等待（毫秒），防止 API Gateway 超时。默认 25000ms。
	MaxWaitMs int `json:"maxWaitMs,omitempty"`
}

type apiResponse struct {
	ExecutionArn string          `json:"executionArn,omitempty"`
	Status       string          `json:"status"`
	TotalMs      int64           `json:"totalMs"`
	Output       json.RawMessage `json:"output,omitempty"`
	Error        string          `json:"error,omitempty"`
}

var (
	initOnce sync.Once
	initErr  error

	sfnClient *sfn.Client
)

func initAWS() {
	initOnce.Do(func() {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			initErr = fmt.Errorf("load aws config: %w", err)
			return
		}
		sfnClient = sfn.NewFromConfig(cfg)
	})
}

func jsonResp(status int, v any) (events.APIGatewayProxyResponse, error) {
	b, _ := json.Marshal(v)
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(b),
	}, nil
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func effectiveTimeout(ctx context.Context, requested time.Duration) time.Duration {
	// API Gateway 最大 29s，Lambda 本函数 Timeout 30s；默认目标：25s。
	// 如果 Lambda context 有更早 deadline，优先以 deadline 为准（并留一点余量）。
	if requested <= 0 {
		requested = 25 * time.Second
	}
	if requested > 28*time.Second {
		requested = 28 * time.Second
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		return requested
	}
	remaining := time.Until(deadline) - 250*time.Millisecond
	if remaining <= 0 {
		return 0
	}
	if remaining < requested {
		return remaining
	}
	return requested
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	initAWS()
	if initErr != nil {
		return jsonResp(500, apiResponse{Status: "ERROR", Error: initErr.Error()})
	}

	smArn := strings.TrimSpace(os.Getenv("STATE_MACHINE_ARN"))
	if smArn == "" {
		return jsonResp(500, apiResponse{Status: "ERROR", Error: "missing env STATE_MACHINE_ARN"})
	}

	var body apiRequest
	if strings.TrimSpace(req.Body) != "" {
		if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
			return jsonResp(400, apiResponse{Status: "ERROR", Error: fmt.Sprintf("invalid json body: %v", err)})
		}
	}

	if strings.TrimSpace(body.RunID) == "" {
		body.RunID = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	body.DelaySeconds = clampInt(body.DelaySeconds, 0, 900)
	if body.MessageBodyBytes < 0 {
		body.MessageBodyBytes = 0
	}

	maxWait := 25 * time.Second
	if body.MaxWaitMs > 0 {
		maxWait = time.Duration(body.MaxWaitMs) * time.Millisecond
	}
	maxWait = effectiveTimeout(ctx, maxWait)
	if maxWait <= 0 {
		return jsonResp(504, apiResponse{Status: "TIMEOUT", Error: "deadline too close"})
	}

	callCtx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	inputBytes, _ := json.Marshal(map[string]any{
		"runId":            body.RunID,
		"delaySeconds":     body.DelaySeconds,
		"messageBodyBytes": body.MessageBodyBytes,
	})

	start := time.Now()
	startOut, err := sfnClient.StartExecution(callCtx, &sfn.StartExecutionInput{
		StateMachineArn: aws.String(smArn),
		Input:           aws.String(string(inputBytes)),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return jsonResp(504, apiResponse{Status: "TIMEOUT", Error: err.Error()})
		}
		return jsonResp(502, apiResponse{Status: "ERROR", Error: fmt.Sprintf("start execution: %v", err)})
	}

	execArn := aws.ToString(startOut.ExecutionArn)
	if execArn == "" {
		return jsonResp(502, apiResponse{Status: "ERROR", Error: "missing executionArn"})
	}

	// Standard workflow 没有 StartSyncExecution：通过 DescribeExecution 轮询等待完成。
	// 注意：轮询间隔要小心，避免频繁打 API；这里用轻量退避。
	interval := 50 * time.Millisecond
	for {
		if callCtx.Err() != nil {
			elapsed := time.Since(start).Milliseconds()
			return jsonResp(504, apiResponse{ExecutionArn: execArn, TotalMs: elapsed, Status: "TIMEOUT", Error: callCtx.Err().Error()})
		}
		desc, err := sfnClient.DescribeExecution(callCtx, &sfn.DescribeExecutionInput{ExecutionArn: aws.String(execArn)})
		if err != nil {
			elapsed := time.Since(start).Milliseconds()
			return jsonResp(502, apiResponse{ExecutionArn: execArn, TotalMs: elapsed, Status: "ERROR", Error: fmt.Sprintf("describe execution: %v", err)})
		}

		s := desc.Status
		if s == sfntypes.ExecutionStatusSucceeded {
			elapsed := time.Since(start).Milliseconds()
			var out json.RawMessage
			if desc.Output != nil {
				out = json.RawMessage([]byte(aws.ToString(desc.Output)))
			}
			return jsonResp(200, apiResponse{ExecutionArn: execArn, TotalMs: elapsed, Status: string(s), Output: out})
		}
		if s == sfntypes.ExecutionStatusFailed || s == sfntypes.ExecutionStatusAborted || s == sfntypes.ExecutionStatusTimedOut {
			elapsed := time.Since(start).Milliseconds()
			msg := aws.ToString(desc.Cause)
			if msg == "" {
				msg = aws.ToString(desc.Error)
			}
			return jsonResp(500, apiResponse{ExecutionArn: execArn, TotalMs: elapsed, Status: string(s), Error: msg})
		}

		time.Sleep(interval)
	}
}

func main() {
	lambda.Start(handler)
}
