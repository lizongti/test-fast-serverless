# 项目文档

## 设计目标

- 创建一个 Dispatcher Lambda 函数，用于向 Push SQS 队列发送数据,发送完成以后，使用长轮询监听Receive SQS队列
- 创建一个 Push SQS 队列，名称TestFastServerlessPush，使用 SQS 触发器触发 Worker Lambda 函数
- 创建一个 Receive SQS 队列，名称TestFastServerlessReceive，用于接收 Worker Lambda 函数处理完成后的回调消息，没有任何触发器。
- 创建一个 Workder Lambda 函数，消费 Push SQS 消息，处理后向 Receive SQS 队列发送回调消息
- 创建一个 API Gateway，触发 Dispatcher Lambda 流程的执行，并同步等待流程完成后返回
- 编写一个 fast_serverless_test.go 测试模块，用于测试整个系统的功能和性能,从 callback Output 的时间戳计算.
- 生成一个 Readme.md 来讲解如何使用该项目，并画图展示程序架构。

## 编写要求

- 使用 sam 进行构建和部署，使用Dockerfile进行本地构建
- 使用 Go 语言编写 Lambda 函数
- 使用 AWS SDK for Go v2
- SQS 和 Lambda 函数必须在同一个 AWS Region（`aws_region`）
- Lambda 架构需要与镜像构建架构一致（`x86_64`/`arm64` 均可）；本项目已在模板里做成参数 `FunctionArchitecture` 可配置，避免不匹配导致运行时报错。
- 在测试日志中增加每条消息发送和接收的时间戳和消息队列名字。
- 将 AWS SDK 客户端移到 handler 外部初始化，可减少热启动时的开销。
- 参照现有代码的风格进行编写，包括命名规范、错误处理和日志记录等。新生成代码以后，删除多余文件。

## 执行约定

- Windows 平台下统一使用 WSL（wsl.exe -e bash -lc）；macOS 平台下统一使用 zsh。
- 允许根据需求创建所需的AWS资源
- 需要自行调用sam build && sam deploy && ./tests.sh完成测试

## 测试用例

- 测试整个流程是否能够正确执行
- 测试整个流程响应时间，完成整个状态机后进行下一次测试。一共测试 10 次，计算平均响应时间
