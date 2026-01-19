// Lambda #2 (Worker)
//
// 作用：由 SQS 触发消费请求消息，并回调 Step Functions（SendTaskSuccess/Failure）。
// 触发方式：SQS Event Source Mapping（RequestQueue -> Lambda）。
// 输出：通过 callback Output（JSON）把各阶段时间戳传回上游（Test/ApiFunction）。
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
)

type msgBody struct {
	ID           string `json:"id"`
	SendUnixNano      int64  `json:"sendUnixNano"`
	SendStartUnixNano int64  `json:"sendStartUnixNano"`
	RunID             string `json:"runId"`
	TaskToken         string `json:"taskToken"`
}

type callbackOutput struct {
	ID        string `json:"id"`
	RunID     string `json:"runId"`
	QueueName string `json:"queueName"`
	Region    string `json:"region"`

	SendUnixNano      int64 `json:"sendUnixNano"`
	SendStartUnixNano int64 `json:"sendStartUnixNano"`
	ReceiveUnixNano   int64 `json:"receiveUnixNano"`
	WorkerDoneUnixNano int64 `json:"workerDoneUnixNano"`

	// 回调请求发起的时间戳（注意：callback 的“结束时间”无法通过本次 Output 回传）。
	CallbackRequestUnixNano int64 `json:"callbackRequestUnixNano"`

	SqsSentTimestampMs         int64 `json:"sqsSentTimestampMs"`
	SqsFirstReceiveTimestampMs int64 `json:"sqsFirstReceiveTimestampMs"`
	SqsApproxReceiveCount      int64 `json:"sqsApproxReceiveCount"`
}

var (
	initOnce sync.Once
	initErr  error

	sfnClient *sfn.Client
	ddbClient *dynamodb.Client
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
		sfnClient = sfn.NewFromConfig(cfg)
		ddbClient = dynamodb.NewFromConfig(cfg)
	})
}

func handler(ctx context.Context, event events.SQSEvent) error {
	if initErr != nil {
		return initErr
	}
	tableName := strings.TrimSpace(os.Getenv("TABLE_NAME"))
	if tableName == "" {
		return errors.New("missing env TABLE_NAME")
	}

	for _, record := range event.Records {
		// 每条 record 对应一条 SQS message。
		queueName := queueNameFromArn(record.EventSourceARN)

		var body msgBody
		if err := json.Unmarshal([]byte(record.Body), &body); err != nil {
			return fmt.Errorf("unmarshal message body: %w", err)
		}
		if strings.TrimSpace(body.ID) == "" {
			return errors.New("missing id in message body")
		}
		if strings.TrimSpace(body.TaskToken) == "" {
			return errors.New("missing taskToken in message body")
		}

		// receiveUnixNano：Worker 实际接收到消息并准备落库的时间戳。
		receiveUnixNano := time.Now().UnixNano()

		// SQS 属性时间戳（毫秒）
		sqsSentTimestampMs := parseInt64OrZero(record.Attributes["SentTimestamp"])
		sqsFirstReceiveTimestampMs := parseInt64OrZero(record.Attributes["ApproximateFirstReceiveTimestamp"])
		sqsApproxReceiveCount := parseInt64OrZero(record.Attributes["ApproximateReceiveCount"])

		// DynamoDB 条件更新：用于演示“只有当 status 不存在或为 pending 才更新”。
		if err := performConditionalUpdate(ctx, tableName, body.ID, receiveUnixNano); err != nil {
			// 条件不满足或更新失败不阻断主流程：仍然返回计时结果。
			log.Printf("ddb conditional update failed id=%s: %v", body.ID, err)
		}

		// Worker 输出：回调 Step Functions，解除 waitForTaskToken。
		workerDoneUnixNano := time.Now().UnixNano()
		callbackRequestUnixNano := time.Now().UnixNano()
		outBytes, err := json.Marshal(callbackOutput{
			ID:        body.ID,
			RunID:     body.RunID,
			QueueName: queueName,
			Region:    region,
			SendUnixNano:            body.SendUnixNano,
			SendStartUnixNano:       body.SendStartUnixNano,
			ReceiveUnixNano:         receiveUnixNano,
			WorkerDoneUnixNano:      workerDoneUnixNano,
			CallbackRequestUnixNano: callbackRequestUnixNano,
			SqsSentTimestampMs:         sqsSentTimestampMs,
			SqsFirstReceiveTimestampMs: sqsFirstReceiveTimestampMs,
			SqsApproxReceiveCount:      sqsApproxReceiveCount,
		})
		if err != nil {
			return fmt.Errorf("marshal callback output: %w", err)
		}
		_, err = sfnClient.SendTaskSuccess(ctx, &sfn.SendTaskSuccessInput{
			TaskToken: aws.String(body.TaskToken),
			Output:    aws.String(string(outBytes)),
		})
		if err != nil {
			return fmt.Errorf("send task success: %w", err)
		}
		log.Printf("sent task success id=%s queue=%s", body.ID, queueName)
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

func performConditionalUpdate(ctx context.Context, tableName, id string, receiveUnixNano int64) error {
	_, err := ddbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]dynamodbtypes.AttributeValue{
			"id": &dynamodbtypes.AttributeValueMemberS{Value: id},
		},
		UpdateExpression:    aws.String("SET #status = :processing, #receiveTime = :receiveTime"),
		ConditionExpression: aws.String("attribute_not_exists(#status) OR #status = :pending"),
		ExpressionAttributeNames: map[string]string{
			"#status":      "status",
			"#receiveTime": "receiveUnixNano",
		},
		ExpressionAttributeValues: map[string]dynamodbtypes.AttributeValue{
			":processing":  &dynamodbtypes.AttributeValueMemberS{Value: "processing"},
			":pending":     &dynamodbtypes.AttributeValueMemberS{Value: "pending"},
			":receiveTime": &dynamodbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", receiveUnixNano)},
		},
	})
	return err
}

func main() {
	initAWS()
	lambda.Start(handler)
}
