// Lambda #1 (Dispatcher)
//
// 作用：API Gateway 后端，向 Push SQS 队列发送请求消息；随后对 Receive SQS 队列做长轮询，
//
//	等待 Worker 回写的回调消息并同步返回。
//
// 对应 SAM 资源：template.yaml 中的 DispatcherFunction
// 环境变量：
//   - PUSH_QUEUE_URL
//   - RECEIVE_QUEUE_URL
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type apiRequest struct {
	RunID            string `json:"runId,omitempty"`
	DelaySeconds     int    `json:"delaySeconds,omitempty"`
	MessageBodyBytes int    `json:"messageBodyBytes,omitempty"`
	MaxWaitMs        int    `json:"maxWaitMs,omitempty"`
}

type apiResponse struct {
	Status  string          `json:"status"`
	TotalMs int64           `json:"totalMs"`
	Output  json.RawMessage `json:"output,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type dispatcherOutput struct {
	RunID string `json:"runId"`
	ID    string `json:"id"`

	Region           string `json:"region"`
	PushQueueName    string `json:"pushQueueName"`
	ReceiveQueueName string `json:"receiveQueueName"`

	DispatchStartUnixNano int64 `json:"dispatchStartUnixNano"`
	SendUnixNano          int64 `json:"sendUnixNano"`
	SendStartUnixNano     int64 `json:"sendStartUnixNano"`
	SendEndUnixNano       int64 `json:"sendEndUnixNano"`

	PollStartUnixNano int64 `json:"pollStartUnixNano"`
	PollEndUnixNano   int64 `json:"pollEndUnixNano"`

	// Worker 回写的时间戳与元数据
	WorkerReceiveUnixNano      int64 `json:"workerReceiveUnixNano"`
	WorkerDoneUnixNano         int64 `json:"workerDoneUnixNano"`
	CallbackSendStartUnixNano  int64 `json:"callbackSendStartUnixNano"`
	CallbackSendEndUnixNano    int64 `json:"callbackSendEndUnixNano"`
	ReceiveMessageUnixNano     int64 `json:"receiveMessageUnixNano"`
	SqsSentTimestampMs         int64 `json:"sqsSentTimestampMs"`
	SqsFirstReceiveTimestampMs int64 `json:"sqsFirstReceiveTimestampMs"`
	SqsApproxReceiveCount      int64 `json:"sqsApproxReceiveCount"`
}

type msgBody struct {
	ID                string `json:"id"`
	SendUnixNano      int64  `json:"sendUnixNano"`
	SendStartUnixNano int64  `json:"sendStartUnixNano"`
	RunID             string `json:"runId"`
	Padding           string `json:"padding,omitempty"`
}

type callbackMessage struct {
	ID    string `json:"id"`
	RunID string `json:"runId"`

	Region           string `json:"region"`
	PushQueueName    string `json:"pushQueueName"`
	ReceiveQueueName string `json:"receiveQueueName"`

	SendUnixNano      int64 `json:"sendUnixNano"`
	SendStartUnixNano int64 `json:"sendStartUnixNano"`

	WorkerReceiveUnixNano     int64 `json:"workerReceiveUnixNano"`
	WorkerDoneUnixNano        int64 `json:"workerDoneUnixNano"`
	CallbackSendStartUnixNano int64 `json:"callbackSendStartUnixNano"`
	CallbackSendEndUnixNano   int64 `json:"callbackSendEndUnixNano"`

	SqsSentTimestampMs         int64 `json:"sqsSentTimestampMs"`
	SqsFirstReceiveTimestampMs int64 `json:"sqsFirstReceiveTimestampMs"`
	SqsApproxReceiveCount      int64 `json:"sqsApproxReceiveCount"`
}

var (
	initOnce sync.Once
	initErr  error

	awsCfg    = struct{ Region string }{}
	sqsClient *sqs.Client
)

func initAWS() {
	initOnce.Do(func() {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			initErr = fmt.Errorf("load aws config: %w", err)
			return
		}
		awsCfg.Region = cfg.Region
		sqsClient = sqs.NewFromConfig(cfg)
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
	// API Gateway 最大 29s；本函数 Timeout 30s；默认目标：25s。
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

	pushQueueURL := strings.TrimSpace(os.Getenv("PUSH_QUEUE_URL"))
	if pushQueueURL == "" {
		return jsonResp(500, apiResponse{Status: "ERROR", Error: "missing env PUSH_QUEUE_URL"})
	}
	receiveQueueURL := strings.TrimSpace(os.Getenv("RECEIVE_QUEUE_URL"))
	if receiveQueueURL == "" {
		return jsonResp(500, apiResponse{Status: "ERROR", Error: "missing env RECEIVE_QUEUE_URL"})
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

	pushQueueName := queueNameFromURL(pushQueueURL)
	receiveQueueName := queueNameFromURL(receiveQueueURL)

	messageID := randHex(16)
	dispatchStart := time.Now().UnixNano()
	sendUnixNano := time.Now().UnixNano()
	sendStart := time.Now().UnixNano()

	bodyObj := msgBody{
		ID:                messageID,
		SendUnixNano:      sendUnixNano,
		SendStartUnixNano: sendStart,
		RunID:             body.RunID,
		Padding:           makePadding(body.MessageBodyBytes),
	}
	bodyBytes, _ := json.Marshal(bodyObj)

	_, err := sqsClient.SendMessage(callCtx, &sqs.SendMessageInput{
		QueueUrl:     &pushQueueURL,
		MessageBody:  awsString(string(bodyBytes)),
		DelaySeconds: int32(body.DelaySeconds),
	})
	sendEnd := time.Now().UnixNano()
	if err != nil {
		return jsonResp(502, apiResponse{Status: "ERROR", Error: fmt.Sprintf("send message: %v", err)})
	}

	pollStart := time.Now().UnixNano()
	cb, receiveMessageUnixNano, pollEnd, err := pollForCallback(callCtx, receiveQueueURL, body.RunID, messageID)
	if err != nil {
		elapsed := (time.Now().UnixNano() - dispatchStart) / int64(time.Millisecond)
		code := 500
		status := "ERROR"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = 504
			status = "TIMEOUT"
		}
		return jsonResp(code, apiResponse{Status: status, TotalMs: elapsed, Error: err.Error()})
	}

	outBytes, _ := json.Marshal(dispatcherOutput{
		RunID:                      body.RunID,
		ID:                         messageID,
		Region:                     awsCfg.Region,
		PushQueueName:              pushQueueName,
		ReceiveQueueName:           receiveQueueName,
		DispatchStartUnixNano:      dispatchStart,
		SendUnixNano:               sendUnixNano,
		SendStartUnixNano:          sendStart,
		SendEndUnixNano:            sendEnd,
		PollStartUnixNano:          pollStart,
		PollEndUnixNano:            pollEnd,
		ReceiveMessageUnixNano:     receiveMessageUnixNano,
		WorkerReceiveUnixNano:      cb.WorkerReceiveUnixNano,
		WorkerDoneUnixNano:         cb.WorkerDoneUnixNano,
		CallbackSendStartUnixNano:  cb.CallbackSendStartUnixNano,
		CallbackSendEndUnixNano:    cb.CallbackSendEndUnixNano,
		SqsSentTimestampMs:         cb.SqsSentTimestampMs,
		SqsFirstReceiveTimestampMs: cb.SqsFirstReceiveTimestampMs,
		SqsApproxReceiveCount:      cb.SqsApproxReceiveCount,
	})

	elapsedMs := (time.Now().UnixNano() - dispatchStart) / int64(time.Millisecond)
	return jsonResp(200, apiResponse{Status: "OK", TotalMs: elapsedMs, Output: outBytes})
}

func awsString(s string) *string { return &s }

func pollForCallback(ctx context.Context, receiveQueueURL string, runID string, id string) (callbackMessage, int64, int64, error) {
	for {
		if ctx.Err() != nil {
			return callbackMessage{}, 0, 0, ctx.Err()
		}
		out, err := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &receiveQueueURL,
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     20,
			VisibilityTimeout:   10,
		})
		pollEnd := time.Now().UnixNano()
		if err != nil {
			return callbackMessage{}, 0, pollEnd, fmt.Errorf("receive message: %w", err)
		}
		if len(out.Messages) == 0 {
			continue
		}
		m := out.Messages[0]
		receiveMessageUnixNano := time.Now().UnixNano()

		var cb callbackMessage
		if m.Body != nil {
			if err := json.Unmarshal([]byte(*m.Body), &cb); err != nil {
				// 无法解析的消息：不阻塞；删除避免毒消息反复出现。
				if m.ReceiptHandle != nil {
					_, _ = sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: &receiveQueueURL, ReceiptHandle: m.ReceiptHandle})
				}
				continue
			}
		}

		if strings.TrimSpace(cb.RunID) == runID && strings.TrimSpace(cb.ID) == id {
			if m.ReceiptHandle != nil {
				_, _ = sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: &receiveQueueURL, ReceiptHandle: m.ReceiptHandle})
			}
			return cb, receiveMessageUnixNano, pollEnd, nil
		}

		// 非本次请求的回调：不删除，立即释放可见性，避免影响并发请求。
		if m.ReceiptHandle != nil {
			_, _ = sqsClient.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
				QueueUrl:          &receiveQueueURL,
				ReceiptHandle:     m.ReceiptHandle,
				VisibilityTimeout: 0,
			})
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func queueNameFromURL(queueURL string) string {
	base := strings.SplitN(queueURL, "?", 2)[0]
	return path.Base(base)
}

func makePadding(extraBytes int) string {
	if extraBytes <= 0 {
		return ""
	}
	pad := make([]byte, extraBytes)
	for i := range pad {
		pad[i] = 'x'
	}
	return string(pad)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func main() {
	lambda.Start(handler)
}
