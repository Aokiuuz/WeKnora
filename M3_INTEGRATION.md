# M3 可复现实验集成契约

里程碑 3（Milestone 3，M3）在 `integration/evaluation-m3-reproducible-experiments` 分支提供可复现实验能力。
M3 使用 M2 的 `evaluation_tasks` 作为任务状态权威来源，并增加不可变数据集版本、实验清单和逐问题结果。

## 1. 数据权威与标识

评测执行从规范化数据库记录读取数据集。数据集由 `evaluation_datasets` 保存身份和可见范围，
`evaluation_dataset_versions` 保存不可变版本，passage、question 和 relevance 三类子表保存版本内容。
段落标识（Passage ID，PID）标识语料段落，问题标识（Question ID，QID）标识评测问题；公开接口使用全局唯一
`dataset_version_id`，`version_number` 只表示同一数据集内的展示序号。

数据集版本同时保存制品哈希和内容哈希。制品哈希校验导入字节，内容哈希使用安全哈希算法 256 位
（Secure Hash Algorithm 256-bit，SHA-256）校验规范化逻辑内容。内容哈希算法按 PID、QID、QID/PID
稳定排序，使用禁用超文本标记语言（HyperText Markup Language，HTML）转义的 JavaScript 对象表示法行
（JavaScript Object Notation Lines，JSON Lines）序列化，并保留原始 8 位统一码转换格式
（8-bit Unicode Transformation Format，UTF-8）文本。

内置 `default` 数据集由 `dataset/samples` 下的五个 Parquet 文件嵌入服务端。启动序列在任务恢复器启动前完成注册，
其制品 SHA-256 固定为 `d598d48018f1da0705920f4432b68a9309c391adbe1403bb1278771b22fe923f`。

## 2. 实验清单

新任务进入 Pending 前生成 schema version 1 的实验清单。清单冻结以下信息：

- 数据集 ID、版本 ID、版本号、制品 SHA-256 和内容 SHA-256。
- 创建请求中的来源知识库（Knowledge Base，KB）ID。
- embedding、chat、rerank 和 summary 四个模型角色及其去密行为配置指纹。
- 检索、重排、生成、随机种子、分块、索引和指标计划。
- 代码版本、提交、构建时间、Go 版本、操作系统、架构、版本形态和数据库驱动。
- 可复现级别与告警。

`evaluation_tasks` 使用 `dataset_version_id`、`dataset_content_sha256`、`experiment_snapshot` 和
`experiment_sha256` 保存清单及其规范 SHA-256。运行阶段重新读取数据集版本并复算内容哈希，同时重新获取模型并校验行为配置指纹。
任一校验不一致都会使任务明确失败。

M3 定义指标计划与观测的唯一数据传输对象（Data Transfer Object，DTO）：
`EvaluationMetricPlanSnapshot`、`EvaluationMetricSpecSnapshot` 和 `EvaluationMetricObservationSnapshot`。
指标实例 ID 使用 `key@version#config_sha256`。里程碑 4（Milestone 4，M4）直接读取这些 DTO；
里程碑 5（Milestone 5，M5）通过适配器生成这些 DTO。

## 3. 执行与发布流程

下图展示任务从请求到可审计结果的当前数据流。图中的数据库事务同时写入逐问题事实和 M2 任务进度，确保两者保持一致。

```mermaid
flowchart LR
    A[创建请求] --> B[解析数据集版本与模型]
    B --> C[冻结实验清单与哈希]
    C --> D[写入 Pending 任务]
    D --> E[读取冻结数据集版本]
    E --> F[导入完整 corpus]
    F --> G[按 sample_index 执行]
    G --> H[事务发布逐问题结果]
    H --> I[更新 finished、metric 与 version]
    I --> J[发布稳定终态]
```

执行器使用冻结的 `dataset_version_id` 读取完整 corpus，并按 `sample_index ASC` 处理问题。相关性边中的字符串 PID
按照版本内 passage 的规范顺序映射为运行期整数 PID。检索与重排结果保存原始排名；未知来源和重复来源保留 `pid=-1`。

`PublishQuestionResult` 在单个事务中插入逐问题结果并更新 `finished`、聚合指标、心跳、租约和任务版本。条件包含租户、任务、
所有者、Running 状态、预期版本、有效租约及 `cancel_requested_at IS NULL`。相同结果哈希的重试幂等；相同样本序号的不同结果哈希返回冲突。
取消请求与问题结果发布在同一数据库真值上竞争，失败事务不会留下逐问题行或进度增量。

## 4. 随机种子与模型指纹

创建应用程序编程接口（Application Programming Interface，API）使用指针区分未提供 seed 与显式 `seed=0`。
OpenAI-compatible 和 Ollama 请求实际携带 seed；Anthropic 对显式 seed 返回类型化不支持错误，
超文本传输协议（Hypertext Transfer Protocol，HTTP）状态码映射为 422。
实验清单记录 `seed`、`seed_provided` 和 `seed_support`。

模型指纹覆盖影响输出的行为配置，并排除应用程序编程接口密钥（Application Programming Interface Key，API Key）、
应用密钥（AppSecret）、应用标识（AppID）、自定义授权头、统一资源定位符（Uniform Resource Locator，URL）用户信息和敏感扩展配置。
运行期间的模型指纹校验保证一个 Success 任务只使用同一组冻结配置。

## 5. 权限、分页与保留

数据集、版本、任务和逐问题结果均按租户边界读取。系统数据集对所有租户可见，租户数据集只对所有者可见；跨租户对象与缺失对象使用相同的 NotFound 语义。

受知识库范围限制的 API Key 只使用实验清单中的 `source_knowledge_base_id` 判断访问范围。来源为空、来源不完整或超出允许范围时，
详情、取消、列表和逐问题查询返回 NotFound 语义。临时评测 KB ID 不参与授权。

逐问题分页按 `sample_index ASC` 使用不透明键集游标，默认页大小为 100，最大值为 500。`evaluation_question_results`
通过 `(tenant_id, task_id)` 复合外键绑定任务并设置 `ON DELETE CASCADE`，M2 保留清理物理删除任务时同步删除逐问题事实。

## 6. API 与 Go SDK

Go 软件开发工具包（Software Development Kit，SDK）位于独立 `client` 模块。M3 增加以下接口：

| 能力 | API | SDK |
| --- | --- | --- |
| 创建数据集 | `POST /api/v1/evaluation/datasets` | `CreateEvaluationDataset` |
| 创建不可变版本 | `POST /api/v1/evaluation/datasets/:id/versions` | `CreateEvaluationDatasetVersion` |
| 数据集列表 | `GET /api/v1/evaluation/datasets` | `ListEvaluationDatasets` |
| 版本列表 | `GET /api/v1/evaluation/datasets/:id/versions` | `ListEvaluationDatasetVersions` |
| 创建实验 | `POST /api/v1/evaluation` | `StartEvaluation` |
| 逐问题分页 | `GET /api/v1/evaluation/tasks/:task_id/questions` | `ListEvaluationQuestionResults` |

创建实验支持 `dataset_version_id`、`configuration` 和 seed。任务详情包含 `experiment` 与
`provenance_complete`。现有 SDK 创建、详情、列表、取消和删除方法保持可用。

## 7. 双数据库迁移

PostgreSQL 使用 `000094` 至 `000096`，SQLite 使用 `000017` 至 `000019`：

| 数据库能力 | PostgreSQL | SQLite |
| --- | ---: | ---: |
| 数据集注册表 | 000094 | 000017 |
| 实验清单列 | 000095 | 000018 |
| 逐问题结果 | 000096 | 000019 |

连续迁移回归覆盖 PostgreSQL `0→96→93→89→93→96` 和 SQLite `0→19→16→12→16→19`。
M3 down 迁移保留 M2 的取消列、活动租约约束、列表索引和保留索引。

## 8. 确定性回归

`dataset/golden/v1` 保存可审阅的固定输入和期望值。回归覆盖 12 项检索与生成指标、相关和无关 PID、未知来源、
重复 PID、重排优先及搜索回退。内容 SHA-256 为
`d8deca2c2ab7b77123130788c9082d0998ecbf76c1d8556952703045cc2556e1`，浮点容差为 `1e-9`。
测试不调用真实模型或网络，并连续 100 次产生相同结果。
