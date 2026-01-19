package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

type execOutput struct {
	ID                      string `json:"id"`
	RunID                   string `json:"runId"`
	QueueName               string `json:"queueName"`
	SendUnixNano            int64  `json:"sendUnixNano"`
	SendStartUnixNano       int64  `json:"sendStartUnixNano"`
	ReceiveUnixNano         int64  `json:"receiveUnixNano"`
	WorkerDoneUnixNano      int64  `json:"workerDoneUnixNano"`
	CallbackRequestUnixNano int64  `json:"callbackRequestUnixNano"`

	SqsSentTimestampMs         int64  `json:"sqsSentTimestampMs"`
	SqsFirstReceiveTimestampMs int64  `json:"sqsFirstReceiveTimestampMs"`
	SqsApproxReceiveCount      int64  `json:"sqsApproxReceiveCount"`
	Region                     string `json:"region"`
}

type apiResponse struct {
	ExecutionArn string          `json:"executionArn,omitempty"`
	Status       string          `json:"status"`
	TotalMs      int64           `json:"totalMs"`
	Output       json.RawMessage `json:"output,omitempty"`
	Error        string          `json:"error,omitempty"`
}

func callRunAPI(ctx context.Context, apiEndpoint string, payload any, timeout time.Duration) (apiResponse, error) {
	if apiEndpoint == "" {
		return apiResponse{}, fmt.Errorf("missing api endpoint")
	}
	if timeout <= 0 {
		timeout = 28 * time.Second
	}

	// ApiEndpoint output already includes /run, but keep this robust.
	url := apiEndpoint
	if len(url) > 0 && url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return apiResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return apiResponse{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return apiResponse{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiResponse{}, fmt.Errorf("api status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var out apiResponse
	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		return apiResponse{}, fmt.Errorf("unmarshal response: %w (body=%s)", err, string(bodyBytes))
	}
	if out.Status == "ERROR" && out.Error != "" {
		return out, fmt.Errorf("api error: %s", out.Error)
	}
	return out, nil
}

func TestStepFunctionsFlowLatency(t *testing.T) {
	if os.Getenv("RUN_REMOTE_TESTS") != "1" {
		t.Skip("set RUN_REMOTE_TESTS=1 to run remote AWS test")
	}

	stage := getenvDefault("STAGE", "dev")
	stackName := getenvDefault("STACK_NAME", fmt.Sprintf("testsqs-%s", stage))
	repeat := getenvIntDefault("REPEAT", 10)
	if repeat <= 0 {
		repeat = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}

	stateMachineArn, err := resolveStackOutput(ctx, cfg, stackName, "StateMachineArn")
	if err != nil {
		t.Fatalf("resolve StateMachineArn: %v", err)
	}

	apiEndpoint, err := resolveStackOutput(ctx, cfg, stackName, "ApiEndpoint")
	if err != nil {
		t.Fatalf("resolve ApiEndpoint: %v", err)
	}

	latenciesMs := make([]int64, 0, repeat)
	var sumMs int64
	var minMs int64
	var maxMs int64

	var sumSendMs int64
	var sumSqsWaitMs int64
	var sumWorkerMs int64
	var sumOverheadMs int64

	type iterMetric struct {
		Iter        int
		TotalMs     int64
		SendToSqsMs int64
		SqsWaitMs   int64
		WorkerMs    int64
		OverheadMs  int64
		WallMs      int64
		ApiLambdaMs int64
	}
	metrics := make([]iterMetric, 0, repeat)

	var minSendMs, maxSendMs int64
	var minSqsWaitMs, maxSqsWaitMs int64
	var minWorkerMs, maxWorkerMs int64
	var minOverheadMs, maxOverheadMs int64

	for i := 0; i < repeat; i++ {
		runID := fmt.Sprintf("run-%d-%d", i, time.Now().UnixNano())

		startWall := time.Now()
		apiOut, err := callRunAPI(ctx, apiEndpoint, map[string]any{
			"runId":            runID,
			"messageBodyBytes": 0,
			// 避免 API Gateway 29s 超时；默认由 ApiFunction 控制为 25s。
			"maxWaitMs": 25000,
		}, 28*time.Second)
		if err != nil {
			t.Fatalf("call api [%d/%d]: %v", i+1, repeat, err)
		}
		if apiOut.Status != "SUCCEEDED" {
			t.Fatalf("api status not succeeded [%d/%d]: status=%s error=%s", i+1, repeat, apiOut.Status, apiOut.Error)
		}
		execArn := apiOut.ExecutionArn
		if execArn == "" {
			t.Fatalf("api missing executionArn [%d/%d]", i+1, repeat)
		}
		var output execOutput
		if len(apiOut.Output) > 0 {
			_ = json.Unmarshal(apiOut.Output, &output)
		}

		wallMs := time.Since(startWall).Milliseconds()

		// Express + StartSyncExecution：直接以 API Lambda 侧测得的等待时间作为总耗时。
		// 退化：如果 apiOut.TotalMs 不可用，则用墙钟时间。
		latencyMs := apiOut.TotalMs
		if latencyMs <= 0 {
			latencyMs = wallMs
		}
		latenciesMs = append(latenciesMs, latencyMs)
		sumMs += latencyMs
		if i == 0 || latencyMs < minMs {
			minMs = latencyMs
		}
		if i == 0 || latencyMs > maxMs {
			maxMs = latencyMs
		}

		// 分布计时：不再依赖 DynamoDB；全部由“消息 + Worker Output”携带的时间戳计算。
		sqsSentUnixNano := output.SqsSentTimestampMs * int64(time.Millisecond)

		sendToSqsMs := int64(0)
		if output.SendStartUnixNano > 0 && sqsSentUnixNano > 0 {
			sendToSqsMs = nanosToMs(sqsSentUnixNano - output.SendStartUnixNano)
			if sendToSqsMs < 0 {
				sendToSqsMs = 0
			}
		}

		sqsWaitMs := int64(0)
		if output.ReceiveUnixNano > 0 {
			base := sqsSentUnixNano
			if base <= 0 {
				base = output.SendUnixNano
			}
			if base > 0 {
				sqsWaitMs = nanosToMs(output.ReceiveUnixNano - base)
				if sqsWaitMs < 0 {
					sqsWaitMs = 0
				}
			}
		}

		workerMs := int64(0)
		if output.WorkerDoneUnixNano > 0 && output.ReceiveUnixNano > 0 {
			workerMs = nanosToMs(output.WorkerDoneUnixNano - output.ReceiveUnixNano)
			if workerMs < 0 {
				workerMs = 0
			}
		}

		overheadMs := latencyMs - (sendToSqsMs + sqsWaitMs + workerMs)
		if overheadMs < 0 {
			overheadMs = 0
		}

		sumSendMs += sendToSqsMs
		sumSqsWaitMs += sqsWaitMs
		sumWorkerMs += workerMs
		sumOverheadMs += overheadMs

		metrics = append(metrics, iterMetric{
			Iter:        i + 1,
			TotalMs:     latencyMs,
			SendToSqsMs: sendToSqsMs,
			SqsWaitMs:   sqsWaitMs,
			WorkerMs:    workerMs,
			OverheadMs:  overheadMs,
			WallMs:      wallMs,
			ApiLambdaMs: apiOut.TotalMs,
		})
		if i == 0 {
			minSendMs, maxSendMs = sendToSqsMs, sendToSqsMs
			minSqsWaitMs, maxSqsWaitMs = sqsWaitMs, sqsWaitMs
			minWorkerMs, maxWorkerMs = workerMs, workerMs
			minOverheadMs, maxOverheadMs = overheadMs, overheadMs
		} else {
			if sendToSqsMs < minSendMs {
				minSendMs = sendToSqsMs
			}
			if sendToSqsMs > maxSendMs {
				maxSendMs = sendToSqsMs
			}
			if sqsWaitMs < minSqsWaitMs {
				minSqsWaitMs = sqsWaitMs
			}
			if sqsWaitMs > maxSqsWaitMs {
				maxSqsWaitMs = sqsWaitMs
			}
			if workerMs < minWorkerMs {
				minWorkerMs = workerMs
			}
			if workerMs > maxWorkerMs {
				maxWorkerMs = workerMs
			}
			if overheadMs < minOverheadMs {
				minOverheadMs = overheadMs
			}
			if overheadMs > maxOverheadMs {
				maxOverheadMs = overheadMs
			}
		}
	}

	den := float64(len(latenciesMs))
	avgTotalMs := float64(sumMs) / den
	avgSendMs := float64(sumSendMs) / den
	avgSqsWaitMs := float64(sumSqsWaitMs) / den
	avgWorkerMs := float64(sumWorkerMs) / den
	avgOverheadMs := float64(sumOverheadMs) / den

	// 冷启动：将第 1 次迭代单独作为“冷启动样本”输出；其余迭代作为 warm 统计。
	// 说明：这里的“冷启动”是端到端视角（API/Dispatcher/Worker 任一环节冷启动都会体现到总耗时上）。
	cold := iterMetric{}
	if len(metrics) > 0 {
		cold = metrics[0]
	}

	warmCount := 0
	var warmSumTotal, warmSumSend, warmSumWait, warmSumWorker, warmSumOverhead int64
	var warmMinTotal, warmMaxTotal int64
	var warmMinSend, warmMaxSend int64
	var warmMinWait, warmMaxWait int64
	var warmMinWorker, warmMaxWorker int64
	var warmMinOverhead, warmMaxOverhead int64

	for i := 1; i < len(metrics); i++ {
		m := metrics[i]
		warmCount++
		warmSumTotal += m.TotalMs
		warmSumSend += m.SendToSqsMs
		warmSumWait += m.SqsWaitMs
		warmSumWorker += m.WorkerMs
		warmSumOverhead += m.OverheadMs
		if warmCount == 1 {
			warmMinTotal, warmMaxTotal = m.TotalMs, m.TotalMs
			warmMinSend, warmMaxSend = m.SendToSqsMs, m.SendToSqsMs
			warmMinWait, warmMaxWait = m.SqsWaitMs, m.SqsWaitMs
			warmMinWorker, warmMaxWorker = m.WorkerMs, m.WorkerMs
			warmMinOverhead, warmMaxOverhead = m.OverheadMs, m.OverheadMs
		} else {
			if m.TotalMs < warmMinTotal {
				warmMinTotal = m.TotalMs
			}
			if m.TotalMs > warmMaxTotal {
				warmMaxTotal = m.TotalMs
			}
			if m.SendToSqsMs < warmMinSend {
				warmMinSend = m.SendToSqsMs
			}
			if m.SendToSqsMs > warmMaxSend {
				warmMaxSend = m.SendToSqsMs
			}
			if m.SqsWaitMs < warmMinWait {
				warmMinWait = m.SqsWaitMs
			}
			if m.SqsWaitMs > warmMaxWait {
				warmMaxWait = m.SqsWaitMs
			}
			if m.WorkerMs < warmMinWorker {
				warmMinWorker = m.WorkerMs
			}
			if m.WorkerMs > warmMaxWorker {
				warmMaxWorker = m.WorkerMs
			}
			if m.OverheadMs < warmMinOverhead {
				warmMinOverhead = m.OverheadMs
			}
			if m.OverheadMs > warmMaxOverhead {
				warmMaxOverhead = m.OverheadMs
			}
		}
	}

	// 输出：Markdown（写 stdout，避免 go test 为 log 行追加缩进/前缀导致表格看起来不整齐，也便于脚本提取写入 result.md）。
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "stateMachine=%s\napi=%s\n\n", stateMachineArn, apiEndpoint)

	buf.WriteString("### Latency Breakdown (ms)\n\n")
	breakdownHeaders := []string{"iter", "totalMs", "sendToSqsMs", "sqsWaitMs", "workerMs", "overheadMs", "wallMs", "apiLambdaMs"}
	breakdownRight := []bool{true, true, true, true, true, true, true, true}
	breakdownRows := make([][]string, 0, len(metrics))
	for _, m := range metrics {
		breakdownRows = append(breakdownRows, []string{
			fmt.Sprintf("%d", m.Iter),
			fmt.Sprintf("%d", m.TotalMs),
			fmt.Sprintf("%d", m.SendToSqsMs),
			fmt.Sprintf("%d", m.SqsWaitMs),
			fmt.Sprintf("%d", m.WorkerMs),
			fmt.Sprintf("%d", m.OverheadMs),
			fmt.Sprintf("%d", m.WallMs),
			fmt.Sprintf("%d", m.ApiLambdaMs),
		})
	}
	buf.WriteString(formatMarkdownTable(breakdownHeaders, breakdownRight, breakdownRows))

	// 冷启动独立表
	buf.WriteString("\n### Cold Start (iter=1)\n\n")
	coldHeaders := []string{"iter", "totalMs", "sendToSqsMs", "sqsWaitMs", "workerMs", "overheadMs", "wallMs", "apiLambdaMs"}
	coldRight := []bool{true, true, true, true, true, true, true, true}
	coldRows := [][]string{}
	if len(metrics) > 0 {
		coldRows = append(coldRows, []string{
			fmt.Sprintf("%d", cold.Iter),
			fmt.Sprintf("%d", cold.TotalMs),
			fmt.Sprintf("%d", cold.SendToSqsMs),
			fmt.Sprintf("%d", cold.SqsWaitMs),
			fmt.Sprintf("%d", cold.WorkerMs),
			fmt.Sprintf("%d", cold.OverheadMs),
			fmt.Sprintf("%d", cold.WallMs),
			fmt.Sprintf("%d", cold.ApiLambdaMs),
		})
	}
	buf.WriteString(formatMarkdownTable(coldHeaders, coldRight, coldRows))

	// warm summary（排除冷启动）
	buf.WriteString("\n### Warm Summary (iter=2..N)\n\n")
	summaryHeaders := []string{"metric", "totalMs", "sendToSqsMs", "sqsWaitMs", "workerMs", "overheadMs"}
	summaryRight := []bool{false, true, true, true, true, true}
	summaryRows := [][]string{}
	if warmCount > 0 {
		warmDen := float64(warmCount)
		warmAvgTotal := float64(warmSumTotal) / warmDen
		warmAvgSend := float64(warmSumSend) / warmDen
		warmAvgWait := float64(warmSumWait) / warmDen
		warmAvgWorker := float64(warmSumWorker) / warmDen
		warmAvgOverhead := float64(warmSumOverhead) / warmDen
		summaryRows = append(summaryRows,
			[]string{"avg", fmt.Sprintf("%.3f", warmAvgTotal), fmt.Sprintf("%.3f", warmAvgSend), fmt.Sprintf("%.3f", warmAvgWait), fmt.Sprintf("%.3f", warmAvgWorker), fmt.Sprintf("%.3f", warmAvgOverhead)},
			[]string{"min", fmt.Sprintf("%d", warmMinTotal), fmt.Sprintf("%d", warmMinSend), fmt.Sprintf("%d", warmMinWait), fmt.Sprintf("%d", warmMinWorker), fmt.Sprintf("%d", warmMinOverhead)},
			[]string{"max", fmt.Sprintf("%d", warmMaxTotal), fmt.Sprintf("%d", warmMaxSend), fmt.Sprintf("%d", warmMaxWait), fmt.Sprintf("%d", warmMaxWorker), fmt.Sprintf("%d", warmMaxOverhead)},
		)
	} else {
		summaryRows = append(summaryRows, []string{"warm", "n/a", "n/a", "n/a", "n/a", "n/a"})
	}
	buf.WriteString(formatMarkdownTable(summaryHeaders, summaryRight, summaryRows))

	// 保留整体 summary 供对比（包含 cold + warm）
	buf.WriteString("\n### All Summary (iter=1..N)\n\n")
	allRows := [][]string{
		{"avg", fmt.Sprintf("%.3f", avgTotalMs), fmt.Sprintf("%.3f", avgSendMs), fmt.Sprintf("%.3f", avgSqsWaitMs), fmt.Sprintf("%.3f", avgWorkerMs), fmt.Sprintf("%.3f", avgOverheadMs)},
		{"min", fmt.Sprintf("%d", minMs), fmt.Sprintf("%d", minSendMs), fmt.Sprintf("%d", minSqsWaitMs), fmt.Sprintf("%d", minWorkerMs), fmt.Sprintf("%d", minOverheadMs)},
		{"max", fmt.Sprintf("%d", maxMs), fmt.Sprintf("%d", maxSendMs), fmt.Sprintf("%d", maxSqsWaitMs), fmt.Sprintf("%d", maxWorkerMs), fmt.Sprintf("%d", maxOverheadMs)},
	}
	buf.WriteString(formatMarkdownTable(summaryHeaders, summaryRight, allRows))

	// 这两个标记用于 tests.sh 提取内容写入 result.md。
	fmt.Println("===BEGIN_RESULT_MD===")
	fmt.Print(buf.String())
	if !strings.HasSuffix(buf.String(), "\n") {
		fmt.Println()
	}
	fmt.Println("===END_RESULT_MD===")
}

func formatMarkdownTable(headers []string, rightAlign []bool, rows [][]string) string {
	colN := len(headers)
	widths := make([]int, colN)
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i := 0; i < colN && i < len(r); i++ {
			if l := len(r[i]); l > widths[i] {
				widths[i] = l
			}
		}
	}

	var b strings.Builder
	// header
	b.WriteString("|")
	for i, h := range headers {
		cell := padCell(h, widths[i], rightAlign[i])
		b.WriteString(" ")
		b.WriteString(cell)
		b.WriteString(" |")
	}
	b.WriteString("\n")

	// separator
	b.WriteString("|")
	for i := 0; i < colN; i++ {
		n := widths[i]
		if n < 3 {
			n = 3
		}
		if rightAlign[i] {
			// right align: ---:
			b.WriteString(" ")
			b.WriteString(strings.Repeat("-", n-1))
			b.WriteString(":")
			b.WriteString(" |")
		} else {
			// left align: :--- (or just ---)
			b.WriteString(" ")
			b.WriteString(strings.Repeat("-", n))
			b.WriteString(" |")
		}
	}
	b.WriteString("\n")

	// rows
	for _, r := range rows {
		b.WriteString("|")
		for i := 0; i < colN; i++ {
			v := ""
			if i < len(r) {
				v = r[i]
			}
			cell := padCell(v, widths[i], rightAlign[i])
			b.WriteString(" ")
			b.WriteString(cell)
			b.WriteString(" |")
		}
		b.WriteString("\n")
	}

	return b.String()
}

func padCell(s string, width int, right bool) string {
	if len(s) >= width {
		return s
	}
	pad := strings.Repeat(" ", width-len(s))
	if right {
		return pad + s
	}
	return s + pad
}

func nanosToMs(n int64) int64 {
	return n / int64(time.Millisecond)
}

func resolveStackOutput(ctx context.Context, cfg aws.Config, stackName string, outputKey string) (string, error) {
	cfn := cloudformation.NewFromConfig(cfg)
	out, err := cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &stackName})
	if err != nil {
		return "", err
	}
	if len(out.Stacks) == 0 {
		return "", fmt.Errorf("stack not found: %s", stackName)
	}
	for _, o := range out.Stacks[0].Outputs {
		if o.OutputKey != nil && *o.OutputKey == outputKey && o.OutputValue != nil {
			return *o.OutputValue, nil
		}
	}
	return "", fmt.Errorf("output %q not found in stack: %s", outputKey, stackName)
}

func getenvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func getenvIntDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
