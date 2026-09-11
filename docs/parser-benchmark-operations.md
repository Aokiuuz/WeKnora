# 解析引擎测评运行说明

当前测评工作区为 `D:\XINIUNIAO\integrations\WeKnora-parser-benchmark`，主应用工作区为 `D:\XINIUNIAO\integrations\WeKnora-topic3-0.8`。测评包含八个解析引擎，共用固定的 100 页公开文档清单。文档来源、抽样、参考标注与评分定义见 [公开基准子集说明](parser-benchmark-dataset.md)。正式执行进度以逐页证据及报告页面为准；PaddleOCR-VL 已完成单页验证，100 页正式批次正在后台执行，完成后自动生成评分与报告。

## 服务与浏览器入口

下表给出各服务的职责及访问位置。宿主机端口绑定回环地址，容器通过 `weknora-personal_default` 网络访问服务。

| 服务 | 职责 | 当前入口 |
| --- | --- | --- |
| 主应用前端 | 知识库、解析配置与评测工作台 | `http://127.0.0.1:5174` |
| 解析测评报告 | 逐页原文、引擎输出、状态与人工核验入口 | `http://127.0.0.1:18090` |
| 独立文档读取服务 | Builtin、MarkItDown、OpenDataLoader | 容器地址 `weknora-parser-docreader:50051` |
| MinerU 本地 | `pipeline` 完整解析，启用公式与表格 | 主机 `http://127.0.0.1:18081`；容器 `http://weknora-parser-mineru:8000` |
| PaddleOCR-VL 本地 | 版式分析与视觉语言模型区域识别 | 主机 `http://127.0.0.1:18082`；容器 `http://weknora-parser-paddle:8080` |
| WeKnora Cloud、MinerU Cloud、PaddleOCR-VL Cloud | 生产云解析适配器 | 读取当前租户的已有云凭据与解析配置 |

主应用租户 `10000` 已保存 `mineru_endpoint=http://weknora-parser-mineru:8000` 和 `paddleocr_vl_endpoint=http://weknora-parser-paddle:8080`。服务器端请求伪造防护（Server-Side Request Forgery，SSRF）的允许主机已精确加入 `weknora-parser-mineru`、`weknora-parser-paddle`，配置记录位于 `artifacts/parser-benchmark/connection.json`。主应用的其他解析字段、全部现有凭据和数据保留；配置备份位于主应用 `.local-service/backups/parser-connection-20260911T091042Z/`。

2026 年 9 月 11 日的只读检查确认：后端 `http://127.0.0.1:18080/health/ready` 返回超文本传输协议（Hypertext Transfer Protocol，HTTP）状态码 `200`；通过前端访问 `/api/v1/models` 的未登录请求返回 `401`，表示请求已到达鉴权接口；报告的 `/status-summary.json` 返回 `200`。文档解析成功另由真实文件验证记录确认。

主前端评测工作台具备可选报告入口，其地址由构建配置 `VITE_PARSER_BENCHMARK_URL` 控制，例如 `http://127.0.0.1:18090`。入口接受 HTTP 或 HTTPS 地址，未配置时隐藏。个人服务已部署该入口，登录工作台后的实际点击可打开八引擎、100 页报告。部署证据保存在 artifacts/parser-benchmark/deployment.json，登录导航证据保存在 artifacts/parser-benchmark/entry-check/checks.json。

## 启动与现有配置

当前机器的模型、依赖和数据已经准备完成。先启动 Docker Desktop，再在测评工作区执行以下命令恢复已有服务。共享服务脚本等待文档读取服务的远程过程调用健康探针（gRPC Remote Procedure Calls health probe）和报告 HTTP 接口就绪后返回。

```powershell
Set-Location D:\XINIUNIAO\integrations\WeKnora-parser-benchmark
$benchmarkPython = 'C:/Users/liuwe/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/python.exe'
$benchmarkReportPython = 'D:/Python 3.11.9/python.exe'
pwsh -NoProfile -File scripts/parser-benchmark-service.ps1 -Action Start -Python $benchmarkPython
pwsh -NoProfile -File scripts/parser-benchmark-mineru.ps1 -Action Start
pwsh -NoProfile -File scripts/parser-benchmark-paddle.ps1 -Action start
```

新机器的安装、模型下载、版本锁定和接口契约分别见 [MinerU 部署说明](../deploy/parser-benchmark/mineru/README.md) 与 [PaddleOCR-VL 部署说明](../deploy/parser-benchmark/paddle/README.md)。公共模型下载无需新增账号。当前机器的 Python 运行路径只适用于本机；其他机器使用与数据准备说明一致的 Python 3.12 环境。共享服务由 [parser-benchmark-service.ps1](../scripts/parser-benchmark-service.ps1) 管理，主应用与各自托管解析服务由各自脚本管理。

[run-parser-benchmark.py](../scripts/run-parser-benchmark.py) 在真实执行时读取 PostgreSQL 数据库 `WeKnora` 中租户 `10000` 的 `credentials.weknoracloud` 和 `parser_engine_config`，同时从运行中的主应用容器读取 `SYSTEM_AES_KEY`。该密钥用于解密应用保存的高级加密标准（Advanced Encryption Standard，AES）密文。配置通过标准输入传给 Go 执行程序，命令行与报告不保存密钥内容。本地两个端点在运行载荷中固定到上表的容器地址。当前八引擎测评使用这些已有配置，无需再次填写账号或密钥。

[parser-benchmark-cloud-status.py](../scripts/parser-benchmark-cloud-status.py) 只查询云额度元数据，不提交解析文档。它仅输出状态码及供应商实际返回的额度字段；额度字段缺失表示该接口没有提供可见数值。供应商控制台账单仍需人工核对。

## 固定输入与基线程序

下表列出保留的实验资产及其用途。原始文档、模型、凭据和逐页输出分别保存在对应位置，Git 跟踪清单、摘要、运行程序与说明。

| 路径 | 内容 |
| --- | --- |
| `dataset/parser-benchmark/manifest-full.json` | 80 页 OmniDocBench 与 20 页 olmOCR-bench 的固定输入清单 |
| `artifacts/parser-benchmark/data/` | 原始文档、参考标注、官方评测源码和评分依赖 |
| `artifacts/parser-benchmark/bin/parser-benchmark` | 当前冻结的 Linux 基线可执行文件 |
| `artifacts/parser-benchmark/baseline-build-identity.json` | 基线源代码标识与可执行文件的安全散列算法 256 位（Secure Hash Algorithm 256-bit，SHA-256）摘要 |
| `artifacts/parser-benchmark/baseline-source/` | 与基线标识对应的源代码证据 |
| `artifacts/parser-benchmark/runs/baseline-v1/` | 八个引擎的逐页记录、Markdown 输出和评分结果 |
| `artifacts/parser-benchmark/cpu-batch-paddle-v1/` | PaddleOCR-VL 单页清单、执行状态、日志及回执 |
| `artifacts/parser-benchmark/report/` | 浏览器报告及原文预览 |
| `D:\XINIUNIAO\.cache\parser-benchmark\mineru\` | MinerU 模型、缓存与服务输出 |
| `deploy/parser-benchmark/paddle/.runtime/` | PaddleOCR-VL 模型、下载分段与真实验证证据 |

当前基线编译标识为 `86436db4+parser-benchmark`，程序 SHA-256 为 `8ab644eee861c75b4351c603b5f0b5c268725cb020b333a1b50f4de87b37cd40`。复现当前基线直接使用该文件。重新构建使用 [build-parser-benchmark.sh](../scripts/build-parser-benchmark.sh)，通过 `PARSER_BENCHMARK_BINARY` 指定新的文件路径，并通过 `PARSER_BENCHMARK_COMMIT` 标注实际源代码版本；构建脚本拒绝覆盖已存在的目标程序，并核对已保存的分词字典内容。以下命令使用本机已有的 Linux 构建镜像 `weknora-nogit-go:1.26`，在当前测评工作区执行。

```powershell
$benchmarkRoot = (Get-Location).Path
$benchmarkCommit = (git rev-parse HEAD).Trim()
docker run --rm --memory 4g --cpus 4 `
  --mount "type=bind,source=$benchmarkRoot,target=/workspace" -w /workspace `
  -e PARSER_BENCHMARK_BINARY=artifacts/parser-benchmark/bin/parser-benchmark-candidate-v1 `
  -e "PARSER_BENCHMARK_COMMIT=$benchmarkCommit+candidate-v1" `
  weknora-nogit-go:1.26 sh scripts/build-parser-benchmark.sh
```

新构建的运行命令同时指定独立程序和独立结果目录，例如：

```powershell
& $benchmarkPython scripts/run-parser-benchmark.py `
  --manifest dataset/parser-benchmark/manifest-full.json `
  --binary artifacts/parser-benchmark/bin/parser-benchmark-candidate-v1 `
  --output artifacts/parser-benchmark/runs/candidate-v1 `
  --engine builtin
```

该命令要求新程序已构建；添加 `--execute` 后才提交真实解析。当前 PaddleOCR-VL 长批次执行器绑定冻结的基线程序及构建标识文件。新程序的独立实验需要独立构建证据，不能写入 `baseline-v1` 或复用其长批次状态。

## PaddleOCR-VL 长批次与恢复

[run-parser-benchmark-cpu-batch.py](../scripts/run-parser-benchmark-cpu-batch.py) 将固定清单拆成 100 份单页清单，逐页串行提交。默认命令只核验输入、基线程序、清单及后处理计划，不发起推理；`--execute` 启用真实执行。当前批次统一使用 `artifacts/parser-benchmark/cpu-batch-paddle-v1/postprocess-plan.json`。以下先配置评分依赖目录，再执行预检；最后一条命令同时用于首次运行和中断后的恢复。

```powershell
$env:PYTHONPATH = (Resolve-Path artifacts/parser-benchmark/data/runtime-deps).Path
$env:PLAYWRIGHT_BROWSERS_PATH = (Join-Path (Resolve-Path artifacts/parser-benchmark/data).Path chromium)
& $benchmarkPython scripts/run-parser-benchmark-cpu-batch.py --postprocess-json artifacts/parser-benchmark/cpu-batch-paddle-v1/postprocess-plan.json
& $benchmarkPython scripts/run-parser-benchmark-cpu-batch.py --execute --postprocess-json artifacts/parser-benchmark/cpu-batch-paddle-v1/postprocess-plan.json
```

默认参数为 `--manifest dataset/parser-benchmark/manifest-full.json`、`--output artifacts/parser-benchmark/runs/baseline-v1`、`--batch-dir artifacts/parser-benchmark/cpu-batch-paddle-v1` 和 `--build-identity artifacts/parser-benchmark/baseline-build-identity.json`。`--health-timeout` 默认为 300 秒。执行前核验实际容器镜像、模型文件摘要、挂载位置与有效配置，执行期间冻结这些标识。

每页执行程序的外层期限为 30 分钟，生产 PaddleOCR-VL 适配器的 HTTP 超时为 1,000 秒，因此请求可能先由适配器结束等待。客户端退出后，超时或连接故障触发该 Paddle 容器的受控停止、启动及健康检查；模型和结果保留。仍有活动客户端时，批次暂停后续提交。此机制避免同步服务继续处理超时请求时积压新页面。

中断后重新执行上述带 `--execute --postprocess-json` 的完整命令，保持计划文件及其内容一致。已有 `success`、`error`、`empty` 或 `timeout` 记录均需通过摘要核验，核验通过后跳过，失败页面也保留为已执行证据。已提交但没有完整结果的页面标记为 `indeterminate`，不自动重投；确认没有活动客户端后可继续其他待执行页面。进程身份不足以判断是否仍在提交时，执行器暂停并保留全部待执行页，等待运行状态核对。

`batch-state.json` 保存状态及剩余页面，`events.jsonl` 和 `batch.log` 保存事件，`receipts/` 保存逐页证据摘要。`execution-complete.json` 仅在全部 100 页均有完整证据时生成；它不表示全部页面解析成功。`batch-complete.json` 还要求配置的后处理命令全部成功。状态不确定的页面会阻止完整完成标志生成。

后处理计划采用 JavaScript 对象表示法（JavaScript Object Notation，JSON）的参数数组，保存评分、浏览器报告、摘要生成及页面核验的实际命令。评分使用随应用提供的 Python 3.12；浏览器报告和摘要使用 `D:/Python 3.11.9/python.exe`。页面核验使用随应用提供的 Node.js 运行时与 Playwright 浏览器自动化库，绝对路径保存在计划中。执行器直接传递参数数组，冻结计划摘要；恢复时沿用首次执行的同一计划。后处理失败保留解析证据，恢复时从未完成命令继续。

本机中央处理器（Central Processing Unit，CPU）配置下，PaddleOCR-VL 短教材页实测耗时 478.938 秒，约 8 分钟；按该样本机械外推，100 页约为 13 小时量级。页面长度、公式、表格和并行负载会改变耗时，该外推不构成完成时间承诺。容器冷启动峰值约 8.57 GiB，当前内存上限 9 GiB、CPU 上限 6 核；长批次保持单并发，并控制其他重负载。Windows 断电、休眠、Docker 退出或脚本进程结束会中断执行，恢复行为按上述证据规则处理。

当前 Paddle 原生预测器固定本地模型批大小为 1，并忽略请求中的 `repetitionPenalty`、`temperature` 和 `topP`。云端与本地使用同名模型和相同请求字段，仍需分别记录实际后端行为；这不构成相同采样参数的证明。

## 评分、报告与人工核验

评分程序使用冻结的官方代码执行 olmOCR-bench 断言及 OmniDocBench 标准指标。公式字符检测匹配（Character Detection Matching，CDM）尚未执行，当前报告不提供其分数或依赖该指标的官方综合分。具体评测环境和可复用执行回执见 [公开基准子集说明](parser-benchmark-dataset.md)。

两套评分环境分别安装：宿主 Python 3.12 执行数据准备与 olmOCR-bench 断言；`weknora-parser-official-eval` 容器中的 Python 3.10.18 执行 OmniDocBench。官方项目要求 Python `>=3.10,<3.12`，因此容器使用独立的 `/opt/omni-venv`。当前官方环境实测包含 `evaluate==0.4.3` 和 `datasets==2.21.0`。报告生成使用本机 Python 3.11.9，已验证其 `matplotlib==3.8.0`、`numpy==1.26.4` 和 `pypdfium2==5.3.0` 可用；该解释器与 Python 3.12 的评分依赖分别管理。

需要重建标准评分环境时，先按数据准备说明恢复固定源码及宿主评分依赖，再执行以下命令。已有同名容器时复用其挂载数据并启动；现有环境已可用时直接进入评分步骤。

```powershell
$benchmarkArtifacts = (Resolve-Path artifacts/parser-benchmark).Path
$benchmarkEvaluator = docker ps -a --filter 'name=^/weknora-parser-official-eval$' --format '{{.Names}}'
if ($benchmarkEvaluator) {
  docker start weknora-parser-official-eval
} else {
  docker run -d --name weknora-parser-official-eval --memory 2g --cpus 2 `
    --mount "type=bind,source=$benchmarkArtifacts,target=/benchmark/artifacts/parser-benchmark" `
    --entrypoint sleep `
    wechatopenai/weknora-docreader@sha256:b9c4636b65b5d4947d5e09cd311ba6cf37f1f2da37c51d4be2b911d432f12abe infinity
}
docker exec weknora-parser-official-eval python -m venv --system-site-packages /opt/omni-venv
docker cp dataset/parser-benchmark/constraints-official-eval.txt weknora-parser-official-eval:/tmp/constraints-official-eval.txt
docker exec weknora-parser-official-eval /opt/omni-venv/bin/python -m pip install --no-compile --timeout 120 --retries 5 -e /benchmark/artifacts/parser-benchmark/data/upstream/omnidocbench-code -c /tmp/constraints-official-eval.txt
```

官方容器将宿主 `artifacts/parser-benchmark/` 挂载到 `/benchmark/artifacts/parser-benchmark/`。评分程序设置 `--official-container` 时，自动使用 `/benchmark` 作为导出路径前缀，与上述挂载一致。

解析证据更新后，以下命令完成评分、浏览器报告与技术摘要刷新。评分依赖目录和 Chromium 浏览器目录由数据准备流程建立。

```powershell
$env:PYTHONPATH = (Resolve-Path artifacts/parser-benchmark/data/runtime-deps).Path
$env:PLAYWRIGHT_BROWSERS_PATH = (Join-Path (Resolve-Path artifacts/parser-benchmark/data).Path chromium)
& $benchmarkPython scripts/score-parser-benchmark.py --manifest dataset/parser-benchmark/manifest-full.json --runs artifacts/parser-benchmark/runs/baseline-v1 --official-container weknora-parser-official-eval
& $benchmarkReportPython scripts/build-parser-benchmark-report.py
& $benchmarkReportPython scripts/summarize-parser-benchmark.py
```

最后需要实际审核者完成三项工作：

1. 通过 `http://127.0.0.1:18090` 优先核验 `recommended_first_pass` 指定的 20 页内容，检查表格归属、公式符号、阅读顺序、遗漏和重复。
2. 对照供应商控制台核验实际账单和用量，记录核验时间及计费口径。
3. 确认当前账号对应的额度、并发、文件限制和使用条件，并记录供应商未通过接口提供的限制。

内容核验结果写入 `human-review.json` 的审核者、时间、决定与备注，签名绑定对应输入和输出摘要。自动评分与 AI 核验建议保留为辅助证据，人工签名由实际审核者完成。

全部现有凭据、知识库数据、公开样本、模型缓存和实验输出保留。当前准备和执行流程无需用户补充新的账号；实际服务结果、账单与人工内容核验共同构成验收证据。

Windows 正式批次通过系统执行状态请求阻止空闲自动休眠，进程退出时释放该请求。屏幕可关闭；关机、手动休眠和 Docker 停止会中断推理，恢复需先检查逐页证据。
