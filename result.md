# Result

本文件用于集中保存一次/多次远程测试的“可复制粘贴”输出结果（Markdown 表格），便于在不翻测试日志的情况下查看。

## 最新一次结果（示例）

> 说明：下面的示例来自 README 中的样例（`REPEAT=10`）。实际运行时，测试会输出两张表：
>
> - `Latency Breakdown (ms)`：每次迭代的分段耗时
> - `Summary`：avg/min/max

### Summary（示例）

| repeat | avgTotalMs | minTotalMs | maxTotalMs | avgSendToSqsMs | avgSqsWaitMs | avgWorkerMs | avgOverheadMs |
| -----: | ---------: | ---------: | ---------: | -------------: | -----------: | ----------: | ------------: |
|     10 |    492.900 |         99 |       3870 |         11.900 |      202.700 |       0.000 |       278.300 |

## 完整测试输出粘贴区（建议每次覆盖）

将 `go test -v` 输出里那段整块 Markdown（包含 `stateMachine=...` / `api=...` 以及两张表）直接粘贴到这里即可。

（粘贴开始）

stateMachine=
api=

### Latency Breakdown (ms)

| iter | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs | wallMs | apiLambdaMs |
| ---: | ------: | ----------: | --------: | -------: | ---------: | -----: | ----------: |

### Summary

| metric | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs |
| ------ | ------: | ----------: | --------: | -------: | ---------: |

（粘贴结束）

## Run 2026-01-18T16:03:54Z

stateMachine=arn:aws:states:ap-east-1:255491288557:stateMachine:TestStateMachine-ZN6gsMqanxyQ
api=https://zdkkgjqxlk.execute-api.ap-east-1.amazonaws.com/dev/run

### Latency Breakdown (ms)

| iter | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs | wallMs | apiLambdaMs |
| ---: | ------: | ----------: | --------: | -------: | ---------: | -----: | ----------: |
|    1 |     360 |          53 |        53 |        0 |        254 |    615 |         360 |
|    2 |     176 |           1 |        43 |        0 |        132 |    238 |         176 |
|    3 |     104 |           2 |        44 |        0 |         58 |    160 |         104 |
|    4 |     105 |           1 |        36 |        0 |         68 |    157 |         105 |
|    5 |     110 |           1 |        42 |        0 |         67 |    165 |         110 |
|    6 |     111 |           2 |        42 |        0 |         67 |    171 |         111 |
|    7 |     101 |           1 |        37 |        0 |         63 |    154 |         101 |
|    8 |     100 |           1 |        36 |        0 |         63 |    152 |         100 |
|    9 |      96 |           1 |        36 |        0 |         59 |    147 |          96 |
|   10 |     105 |           1 |        37 |        0 |         67 |    154 |         105 |

### Summary

| metric | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs |
| ------ | ------: | ----------: | --------: | -------: | ---------: |
| avg    | 136.800 |       6.400 |    40.600 |    0.000 |     89.800 |
| min    |      96 |           1 |        36 |        0 |         58 |
| max    |     360 |          53 |        53 |        0 |        254 |

## Run 2026-01-18T16:20:10Z

stateMachine=arn:aws:states:ap-east-1:255491288557:stateMachine:TestStateMachine-ZN6gsMqanxyQ
api=https://zdkkgjqxlk.execute-api.ap-east-1.amazonaws.com/dev/run

### Latency Breakdown (ms)

| iter | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs | wallMs | apiLambdaMs |
| ---: | ------: | ----------: | --------: | -------: | ---------: | -----: | ----------: |
|    1 |    2661 |         118 |      1713 |      162 |        668 |   3249 |        2661 |
|    2 |     168 |           2 |        45 |        3 |        118 |    382 |         168 |
|    3 |     114 |           2 |        48 |        5 |         59 |    326 |         114 |
|    4 |     106 |           2 |        37 |        4 |         63 |    322 |         106 |
|    5 |     121 |           1 |        40 |        4 |         76 |    409 |         121 |
|    6 |     146 |          37 |        36 |        3 |         70 |    511 |         146 |
|    7 |     112 |           2 |        35 |        3 |         72 |    409 |         112 |
|    8 |     101 |           2 |        35 |        3 |         61 |    317 |         101 |
|    9 |     108 |           2 |        36 |        4 |         66 |    321 |         108 |
|   10 |     102 |           2 |        36 |        3 |         61 |    403 |         102 |

### Summary

| metric | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs |
| ------ | ------: | ----------: | --------: | -------: | ---------: |
| avg    | 373.900 |      17.000 |   206.100 |   19.400 |    131.400 |
| min    |     101 |           1 |        35 |        3 |         59 |
| max    |    2661 |         118 |      1713 |      162 |        668 |

## Run 2026-01-18T16:22:48Z

stateMachine=arn:aws:states:ap-east-1:255491288557:stateMachine:TestStateMachine-ZN6gsMqanxyQ
api=https://zdkkgjqxlk.execute-api.ap-east-1.amazonaws.com/dev/run

### Latency Breakdown (ms)

| iter | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs | wallMs | apiLambdaMs |
| ---: | ------: | ----------: | --------: | -------: | ---------: | -----: | ----------: |
|    1 |     346 |          53 |        60 |       39 |        194 |    803 |         346 |
|    2 |     164 |           2 |        40 |        5 |        117 |    379 |         164 |
|    3 |     160 |           2 |        37 |        3 |        118 |    431 |         160 |
|    4 |     114 |           2 |        38 |        7 |         67 |    307 |         114 |
|    5 |     110 |           2 |        36 |        4 |         68 |    409 |         110 |
|    6 |     108 |           2 |        36 |        4 |         66 |    409 |         108 |
|    7 |     108 |           2 |        36 |        3 |         67 |    409 |         108 |
|    8 |     110 |           2 |        44 |        3 |         61 |    409 |         110 |
|    9 |     118 |           2 |        43 |        4 |         69 |    403 |         118 |
|   10 |     104 |           2 |        36 |        3 |         63 |    234 |         104 |

### Cold Start (iter=1)

| iter | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs | wallMs | apiLambdaMs |
| ---: | ------: | ----------: | --------: | -------: | ---------: | -----: | ----------: |
|    1 |     346 |          53 |        60 |       39 |        194 |    803 |         346 |

### Warm Summary (iter=2..N)

| metric | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs |
| ------ | ------: | ----------: | --------: | -------: | ---------: |
| avg    | 121.778 |       2.000 |    38.444 |    4.000 |     77.333 |
| min    |     104 |           2 |        36 |        3 |         61 |
| max    |     164 |           2 |        44 |        7 |        118 |

### All Summary (iter=1..N)

| metric | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs |
| ------ | ------: | ----------: | --------: | -------: | ---------: |
| avg    | 144.200 |       7.100 |    40.600 |    7.500 |     89.000 |
| min    |     104 |           2 |        36 |        3 |         61 |
| max    |     346 |          53 |        60 |       39 |        194 |

## Run 2026-01-18T16:50:42Z

stateMachine=arn:aws:states:ap-east-1:255491288557:stateMachine:TestStateMachine-FX6R5pJoc4zo
api=https://zdkkgjqxlk.execute-api.ap-east-1.amazonaws.com/dev/run

### Latency Breakdown (ms)

| iter | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs | wallMs | apiLambdaMs |
| ---: | ------: | ----------: | --------: | -------: | ---------: | -----: | ----------: |
|    1 |    1579 |         144 |       205 |      155 |       1075 |   3119 |        1579 |
|    2 |     405 |           1 |        44 |        4 |        356 |    716 |         405 |
|    3 |     400 |           2 |        46 |        5 |        347 |    513 |         400 |
|    4 |     399 |           2 |        38 |        4 |        355 |    457 |         399 |
|    5 |     398 |           1 |        39 |        3 |        355 |    465 |         398 |
|    6 |     400 |           1 |        41 |        4 |        354 |    613 |         400 |
|    7 |     313 |           2 |        38 |        3 |        270 |    510 |         313 |
|    8 |     407 |           1 |        37 |        3 |        366 |    469 |         407 |
|    9 |     403 |           1 |        42 |        4 |        356 |    457 |         403 |
|   10 |     235 |           1 |        37 |        4 |        193 |    310 |         235 |

### Cold Start (iter=1)

| iter | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs | wallMs | apiLambdaMs |
| ---: | ------: | ----------: | --------: | -------: | ---------: | -----: | ----------: |
|    1 |    1579 |         144 |       205 |      155 |       1075 |   3119 |        1579 |

### Warm Summary (iter=2..N)

| metric | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs |
| ------ | ------: | ----------: | --------: | -------: | ---------: |
| avg    | 373.333 |       1.333 |    40.222 |    3.778 |    328.000 |
| min    |     235 |           1 |        37 |        3 |        193 |
| max    |     407 |           2 |        46 |        5 |        366 |

### All Summary (iter=1..N)

| metric | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs |
| ------ | ------: | ----------: | --------: | -------: | ---------: |
| avg    | 493.900 |      15.600 |    56.700 |   18.900 |    402.700 |
| min    |     235 |           1 |        37 |        3 |        193 |
| max    |    1579 |         144 |       205 |      155 |       1075 |

## Run 2026-01-18T16:54:05Z

stateMachine=arn:aws:states:ap-east-1:255491288557:stateMachine:TestStateMachine-FX6R5pJoc4zo
api=https://zdkkgjqxlk.execute-api.ap-east-1.amazonaws.com/dev/run

### Latency Breakdown (ms)

| iter | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs | wallMs | apiLambdaMs |
| ---: | ------: | ----------: | --------: | -------: | ---------: | -----: | ----------: |
|    1 |     613 |          47 |        55 |       56 |        455 |   1954 |         613 |
|    2 |     241 |           2 |        36 |        5 |        198 |    301 |         241 |
|    3 |     298 |           2 |        52 |        4 |        240 |    356 |         298 |
|    4 |     233 |           2 |        38 |        3 |        190 |    329 |         233 |
|    5 |     244 |           2 |        39 |        3 |        200 |    301 |         244 |
|    6 |     240 |           1 |        36 |        4 |        199 |    294 |         240 |
|    7 |     240 |           1 |        37 |        3 |        199 |    295 |         240 |
|    8 |     352 |           2 |        35 |        4 |        311 |    414 |         352 |
|    9 |     238 |           2 |        35 |        3 |        198 |    301 |         238 |
|   10 |     229 |           1 |        36 |        3 |        189 |    301 |         229 |

### Cold Start (iter=1)

| iter | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs | wallMs | apiLambdaMs |
| ---: | ------: | ----------: | --------: | -------: | ---------: | -----: | ----------: |
|    1 |     613 |          47 |        55 |       56 |        455 |   1954 |         613 |

### Warm Summary (iter=2..N)

| metric | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs |
| ------ | ------: | ----------: | --------: | -------: | ---------: |
| avg    | 257.222 |       1.667 |    38.222 |    3.556 |    213.778 |
| min    |     229 |           1 |        35 |        3 |        189 |
| max    |     352 |           2 |        52 |        5 |        311 |

### All Summary (iter=1..N)

| metric | totalMs | sendToSqsMs | sqsWaitMs | workerMs | overheadMs |
| ------ | ------: | ----------: | --------: | -------: | ---------: |
| avg    | 292.800 |       6.200 |    39.900 |    8.800 |    237.900 |
| min    |     229 |           1 |        35 |        3 |        189 |
| max    |     613 |          47 |        55 |       56 |        455 |
