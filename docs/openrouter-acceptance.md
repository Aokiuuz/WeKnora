# OpenRouter 配置与验收

## 本机入口与模型角色

本机服务入口为 `http://127.0.0.1:5174`。运行项目外层的 `启动WeKnora.cmd` 可启动数据库、缓存、文档解析、后端和网页；具体配置见 [个人服务说明](personal-service.md)。

仓库内同时提供 `Start-WeKnora.cmd`、`Stop-WeKnora.cmd` 和 `WeKnora-Status.cmd`，分别执行启动、停止和状态查看。

模型管理中的远程模型使用 `https://openrouter.ai/api/v1`，供应商选择 OpenRouter，密钥填写账户提供的 API Key。API（Application Programming Interface，应用程序编程接口）密钥保存在本机私有配置或加密模型配置中。

| 模型标识 | 用途 | 本次配置 |
| --- | --- | --- |
| `deepseek/deepseek-v4-flash` | 日常问答与真实评测 | 对话模型 |
| `moonshotai/kimi-k2.5` | 问答对照与备用 | 对话模型 |
| `deepseek/deepseek-v4-pro` | 复杂问题与独立自动复核 | 对话模型 |
| `qwen/qwen3.8-flash` | 问答备选 | 对话模型；共享上游存在间歇性限流 |
| `qwen/qwen3-embedding-8b` | 文本向量与检索 | 向量模型，维度 1024，允许维度覆盖 |
| `qwen/qwen3-embedding-4b` | 向量模型备选 | 向量模型，维度 1024，允许维度覆盖 |

知识库绑定一个向量模型。选择向量模型后导入文档，系统生成分块与索引。更换向量模型需要重新生成对应索引。问答模型通过会话或评测配置选择。当前验收使用向量检索与关键词检索，重排模型为空。

## 价格与账本

模型价格页按美元建立生效时间明确的价格版本。OpenRouter 的目录价格来自 [对话模型目录](https://openrouter.ai/api/v1/models) 与 [向量模型目录](https://openrouter.ai/api/v1/embeddings/models)。目录价格可能是多个路由的最低报价，具体请求还记录供应商报告的费用。

成功请求包含 `usage.cost`、模型供应商为 OpenRouter、价格币种为美元时，账本以供应商报告金额结算。金额按四舍五入转换为微美元，即一美元的百万分之一。模型快照的 `billing_usage` 保留原始十进制费用、价格估算值及费用来源。模型身份与价格快照继续使用不可变校验。供应商未报告费用时，账本使用完整的价格及用量条件计算估算值；缺失价格的记录保留未定价状态。

## 零费用回归

```sh
make evaluation-reproduce
python3 scripts/evaluation-rag-http-smoke.py --output artifacts/evaluation-rag-http
python3 scripts/evaluation-fault-http-smoke.py \
  --output artifacts/evaluation-fault-http \
  --server-binary artifacts/evaluation-rag-http/weknora-server
python3 -B scripts/test_evaluation_openrouter_budget.py
```

确定性评测使用固定供应商响应，验证指标计算、导出、缓存恢复和故障状态。真实质量实验单独记录模型、数据版本、费用及供应商请求信息。持续集成（Continuous Integration，CI）工作流执行上述检查；主分支要求 `Deterministic golden evaluation` 检查成功。

## 真实模型验收

真实验收脚本在独立 Linux 容器中使用 SQLite 数据库，API 密钥经标准输入传递。应用仅持有本地代理占位凭据，所有付费请求经过预算代理。运行前准备带 `sqlite_fts5` 构建标记的后端二进制，并配置 Jieba 词典路径。

```sh
go build -buildvcs=false -tags sqlite_fts5 -o /private/bin/WeKnora ./cmd/server
python3 scripts/evaluation-openrouter-acceptance.py \
  --output /private/evidence/run-01 \
  --budget-file /private/evidence/budget.json \
  --server-binary /private/bin/WeKnora
```

脚本从标准输入读取包含 `openrouter_key` 和 `approved_usd: 20` 的 JSON 对象。输出目录必须不存在，父目录必须存在。各批次复用同一个预算文件，跨进程排他锁限制同一时间仅运行一个付费批次。请求发出前写入保守费用预留；供应商报告费用后结算，未确定费用保留预留金额。禁止使用 Python 的 `-O` 参数。

公开数据实验分别使用中文阅读理解数据集 CMRC2018（Chinese Machine Reading Comprehension 2018）与斯坦福问答数据集 SQuAD2（Stanford Question Answering Dataset 2.0）的固定开发集子集。每个子集包含 24 个问题和 32 个候选段落。重启缓存实验使用固定的 32 个段落及其中 2 个问题，运行五组冷启动与重启后复用对照。完整开发集或生产流量上的性能需要对应的数据与实验。

`evaluation-openrouter-probes.py` 通过生产 Wiki 页面提示词和模型适配器执行内容变化失效及 30 组固定前缀对照。两组使用相同资料与生成参数，改变共享资料块的位置，交替请求顺序，并固定上游路由。供应商缓存收益按真实观察值报告。

模型辅助复核使用 `evaluation-openrouter-judge.py`，按问题交替交换匿名答案 A、B 的顺序，将 DeepSeek V4 Pro 的正确性及证据支持性判断单独保存。单个模型评分者的判断需要结合原始答案与证据审查。两个脚本通过 `--probe-binary` 指定使用生产适配器的探测程序：

```sh
go build -buildvcs=false -tags sqlite_fts5 \
  -o .local-service/bin/evaluation-provider-probe ./cmd/evaluation-provider-probe
```

实验输出包括结果 JSON、逗号分隔值（Comma-Separated Values，CSV）文件、数据库、供应商收据与费用预留记录。自动指标和模型辅助复核保留各自身份；人工评分字段只接受真实评分者的结果。
