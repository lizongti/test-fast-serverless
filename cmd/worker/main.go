// Lambda #2 (Worker)
//
// 作用：由 Push SQS 触发消费请求消息，并把处理结果发送到 Receive SQS（回调消息）。
// 触发方式：SQS Event Source Mapping（RequestQueue -> Lambda）。
// 输出：通过 Receive SQS 消息（JSON）把各阶段时间戳传回上游（Dispatcher API）。
//
// 对应 SAM 资源：template.yaml 中的 WorkerFunction
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type msgBody struct {
	ID                string `json:"id"`
	SendUnixNano      int64  `json:"sendUnixNano"`
	SendStartUnixNano int64  `json:"sendStartUnixNano"`
	RunID             string `json:"runId"`
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

	sqsClient *sqs.Client
	region    string
)

func initAWS() {
	initOnce.Do(func() {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			initErr = fmt.Errorf("load aws config: %w", err)
			return
		}
		region = cfg.Region
		sqsClient = sqs.NewFromConfig(cfg)
	})
}

func handler(ctx context.Context, event events.SQSEvent) error {
	initAWS()
	if initErr != nil {
		return initErr
	}
	receiveQueueURL := strings.TrimSpace(os.Getenv("RECEIVE_QUEUE_URL"))
	if receiveQueueURL == "" {
		return errors.New("missing env RECEIVE_QUEUE_URL")
	}
	receiveQueueName := queueNameFromURL(receiveQueueURL)

	for _, record := range event.Records {
		// 每条 record 对应一条 SQS message。
		pushQueueName := queueNameFromArn(record.EventSourceARN)

		var body msgBody
		if err := json.Unmarshal([]byte(record.Body), &body); err != nil {
			return fmt.Errorf("unmarshal message body: %w", err)
		}
		if strings.TrimSpace(body.ID) == "" {
			return errors.New("missing id in message body")
		}
		if strings.TrimSpace(body.RunID) == "" {
			return errors.New("missing runId in message body")
		}

		// workerReceiveUnixNano：Worker 实际开始处理的时间戳。
		workerReceiveUnixNano := time.Now().UnixNano()

		// SQS 属性时间戳（毫秒）
		sqsSentTimestampMs := parseInt64OrZero(record.Attributes["SentTimestamp"])
		sqsFirstReceiveTimestampMs := parseInt64OrZero(record.Attributes["ApproximateFirstReceiveTimestamp"])
		sqsApproxReceiveCount := parseInt64OrZero(record.Attributes["ApproximateReceiveCount"])

		workerDoneUnixNano := time.Now().UnixNano()
		callbackSendStartUnixNano := time.Now().UnixNano()
		cbBytes, err := json.Marshal(callbackMessage{
			ID:                         body.ID,
			RunID:                      body.RunID,
			Region:                     region,
			PushQueueName:              pushQueueName,
			ReceiveQueueName:           receiveQueueName,
			SendUnixNano:               body.SendUnixNano,
			SendStartUnixNano:          body.SendStartUnixNano,
			WorkerReceiveUnixNano:      workerReceiveUnixNano,
			WorkerDoneUnixNano:         workerDoneUnixNano,
			CallbackSendStartUnixNano:  callbackSendStartUnixNano,
			SqsSentTimestampMs:         sqsSentTimestampMs,
			SqsFirstReceiveTimestampMs: sqsFirstReceiveTimestampMs,
			SqsApproxReceiveCount:      sqsApproxReceiveCount,
		})
		if err != nil {
			return fmt.Errorf("marshal callback message: %w", err)
		}
		cbBody := string(cbBytes)
		_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:    &receiveQueueURL,
			MessageBody: &cbBody,
		})
		callbackSendEndUnixNano := time.Now().UnixNano()
		if err != nil {
			return fmt.Errorf("send callback message: %w", err)
		}

		log.Printf("worker processed id=%s pushQueue=%s workerReceiveUnixNano=%d workerDoneUnixNano=%d callbackQueue=%s callbackSendStartUnixNano=%d callbackSendEndUnixNano=%d", body.ID, pushQueueName, workerReceiveUnixNano, workerDoneUnixNano, receiveQueueName, callbackSendStartUnixNano, callbackSendEndUnixNano)
	}

	return nil
}

func queueNameFromArn(arn string) string {
	// arn:aws:sqs:region:account:queueName
	parts := strings.Split(arn, ":")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func parseInt64OrZero(s string) int64 {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func queueNameFromURL(queueURL string) string {
	base := strings.SplitN(queueURL, "?", 2)[0]
	return path.Base(base)
}

func main() {
	lambda.Start(handler)
}
