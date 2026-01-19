# TestServerlessSystem

使用 AWS SAM + Go（AWS SDK for Go v2）部署两个 SQS 队列与两类 Lambda：

- Dispatcher Lambda：由 API Gateway 触发，向 Push 队列发送请求消息；随后长轮询 Receive 队列等待回调消息并同步返回
- Worker Lambda：由 Push 队列触发，消费请求消息并向 Receive 队列发送回调消息（回调中包含各阶段时间戳，用于分布计时）

测试用例通过“调用 API 并顺序执行 10 次”的方式，测量端到端响应时间；并在测试日志中记录每条消息发送/接收的时间戳与队列名。

## 项目结构

- `cmd/dispatcher/main.go`：Dispatcher Lambda（Go）
- `cmd/worker/main.go`：Worker Lambda（Go）
- `fast_serverless_test.go`：远程测试用例（Go test）
- `tests.sh`：便捷测试脚本（设置 env 后执行 go test）

## 架构

```mermaid
flowchart LR
  Client[Client] -->|HTTP POST /run| APIGW[API Gateway]
  APIGW -->|Invoke| Dispatcher[Lambda: Dispatcher]
  Dispatcher -->|SendMessage(request)| PushSQS[(SQS Queue: TestFastServerlessPush)]
  PushSQS -->|Trigger| Worker[Lambda: Worker]
  Worker -->|SendMessage(callback)| ReceiveSQS[(SQS Queue: TestFastServerlessReceive)]
  Dispatcher -->|ReceiveMessage (long poll)| ReceiveSQS
  Dispatcher -->|HTTP 200| Client

  subgraph Region[AWS Region (aws_region)]
    APIGW
    Dispatcher
    PushSQS
    ReceiveSQS
    Worker
  end
```

说明：该项目只要求同一个 AWS Region（`aws_region`），因此模板不强制引入 VPC/子网等网络约束。

注意：`AWS_REGION` 属于 Lambda 保留环境变量，SDK 会从运行时环境自动获取。

注意：本项目按需求固定队列名称：`TestFastServerlessPush` 与 `TestFastServerlessReceive`。

## 前置条件

- 已安装并配置：`aws` CLI（可用凭证、默认 region）
- 已安装：`sam` CLI
- 已安装并运行：Docker（用于 `sam build` 的 Dockerfile 本地构建）

## 构建

在仓库根目录执行：

```bash
sam build
```

## 部署

```bash
sam deploy --guided
```

提示：本项目使用 Image 方式部署（Dockerfile 构建）。如果你不想手动配置 ECR 仓库，可以使用：

```bash
sam deploy --guided --resolve-image-repos
```

## 远程测试（单条消息重复多次）

测试模块采用 Go 的 `_test.go` 形式（不使用 shell）。远程测试默认是 **skip**，避免在无 AWS 凭证/未部署时失败。

在已部署 stack、且本机 AWS 凭证可用时运行：

推荐使用 tests 目录下的便捷脚本（会自动设置必要环境变量并执行 Go 测试用例）：

```bash
chmod +x ./tests.sh
./tests.sh dev
```

说明：如果仓库根目录存在 `samconfig.toml`（`sam deploy --guided` 生成），`tests.sh` 会读取其中的 `stack_name` 作为默认栈名。
（优先使用 `yq` 解析；未安装时会自动回退到文本解析。）

也可以直接用 `go test`（远程测试默认是 **skip**，需要显式设置 `RUN_REMOTE_TESTS=1`）：

```bash
RUN_REMOTE_TESTS=1 STAGE=dev REPEAT=10 go test -run TestFastServerlessFlowLatency -v
```

自定义 stack 与次数：

```bash
RUN_REMOTE_TESTS=1 STACK_NAME=testsqs-dev REPEAT=50 go test -run TestFastServerlessFlowLatency -v
```

## 测试日志输出

测试用例会把每次迭代的耗时拆分输出为 Markdown 表格（不输出时间戳）。

同时会把第 1 次迭代作为“冷启动样本（Cold Start）”单独输出一张表，并对第 2..N 次（Warm）独立统计 avg/min/max。

另外，运行 `./tests.sh` 时会自动把本次测试输出块追加写入 `result.md`，便于沉淀每次运行结果。

## 分布计时（Markdown 表格）

测试输出会包含以下表格：

- `Latency Breakdown (ms)`：每次迭代的分段耗时
- `Cold Start (iter=1)`：冷启动样本（第 1 次迭代）
- `Warm Summary (iter=2..N)`：排除冷启动后的 avg/min/max
- `All Summary (iter=1..N)`：包含全部迭代的 avg/min/max（用于对比）

你可以直接把测试输出里的表复制粘贴到 README 或其他文档里。
