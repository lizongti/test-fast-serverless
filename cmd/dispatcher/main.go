// Lambda #1 (Dispatcher)
//
// 作用：向 SQS 队列发送消息。
// 触发方式：由测试用例或外部调用通过 aws lambda invoke 远程触发。
// 输入/输出：返回每条消息的发送时间戳与队列名，供测试用例计算端到端延迟。
//
// 对应 SAM 资源：template.yaml 中的 DispatcherFunction
// 环境变量：QUEUE_URL（SQS QueueUrl）
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type Request struct {
	TaskToken string `json:"taskToken"`
	Input     struct {
		RunID            string `json:"runId,omitempty"`
		DelaySeconds     int    `json:"delaySeconds,omitempty"`
		MessageBodyBytes int    `json:"messageBodyBytes,omitempty"`
	} `json:"input"`
}

type Response struct {
	QueueName string `json:"queueName"`
	Region    string `json:"region"`

	RunID string `json:"runId"`
	ID    string `json:"id"`

	SendUnixNano      int64 `json:"sendUnixNano"`
	SendStartUnixNano int64 `json:"sendStartUnixNano"`
	SendEndUnixNano   int64 `json:"sendEndUnixNano"`

	ReceiveUnixNano            int64 `json:"receiveUnixNano"`
	WorkerDoneUnixNano         int64 `json:"workerDoneUnixNano"`
	CallbackRequestUnixNano    int64 `json:"callbackRequestUnixNano"`
	SqsSentTimestampMs         int64 `json:"sqsSentTimestampMs"`
	SqsFirstReceiveTimestampMs int64 `json:"sqsFirstReceiveTimestampMs"`
	SqsApproxReceiveCount      int64 `json:"sqsApproxReceiveCount"`
}

type msgBody struct {
	ID                string `json:"id"`
	SendUnixNano      int64  `json:"sendUnixNano"`
	SendStartUnixNano int64  `json:"sendStartUnixNano"`
	RunID             string `json:"runId"`
	TaskToken         string `json:"taskToken"`
	Padding           string `json:"padding,omitempty"`
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

func handler(ctx context.Context, req Request) (Response, error) {
	// Standard workflow：状态机使用 waitForTaskToken；Dispatcher 只负责把 taskToken 放进请求队列，Worker 处理后回调解除阻塞。
	requestQueueURL := os.Getenv("REQUEST_QUEUE_URL")
	if requestQueueURL == "" {
		return Response{}, errors.New("missing env REQUEST_QUEUE_URL")
	}
	if initErr != nil {
		return Response{}, initErr
	}
	if strings.TrimSpace(req.TaskToken) == "" {
		return Response{}, errors.New("missing taskToken in request")
	}
	if req.Input.DelaySeconds < 0 {
		req.Input.DelaySeconds = 0
	}
	if req.Input.DelaySeconds > 900 {
		req.Input.DelaySeconds = 900
	}
	if req.Input.MessageBodyBytes < 0 {
		req.Input.MessageBodyBytes = 0
	}
	if strings.TrimSpace(req.Input.RunID) == "" {
		req.Input.RunID = randHex(12)
	}

	qn := queueNameFromURL(requestQueueURL)

	// 生成消息体：包含唯一 id、发送时间戳；Worker 处理后把结果发回 response queue。
	messageID := randHex(16)
	sendUnixNano := time.Now().UnixNano()
	sendStart := time.Now().UnixNano()

	bodyObj := msgBody{
		ID:                messageID,
		SendUnixNano:      sendUnixNano,
		SendStartUnixNano: sendStart,
		RunID:             req.Input.RunID,
		TaskToken:         req.TaskToken,
		Padding:           makePadding(req.Input.MessageBodyBytes),
	}
	bodyBytes, _ := json.Marshal(bodyObj)
	body := string(bodyBytes)

	_, err := sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:     &requestQueueURL,
		MessageBody:  &body,
		DelaySeconds: int32(req.Input.DelaySeconds),
	})
	sendEnd := time.Now().UnixNano()
	if err != nil {
		return Response{}, fmt.Errorf("send message: %w", err)
	}

	// Lambda 日志：便于排查（测试日志仍由测试用例输出）。
	log.Printf("sent request id=%s queue=%s sendUnixNano=%d sendStartUnixNano=%d sendEndUnixNano=%d", messageID, qn, sendUnixNano, sendStart, sendEnd)

	return Response{
		QueueName:         qn,
		Region:            awsCfg.Region,
		RunID:             req.Input.RunID,
		ID:                messageID,
		SendUnixNano:      sendUnixNano,
		SendStartUnixNano: sendStart,
		SendEndUnixNano:   sendEnd,
	}, nil
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
	initAWS()
	lambda.Start(handler)
}
