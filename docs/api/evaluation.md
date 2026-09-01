# 评测 API

评测应用程序编程接口（Application Programming Interface，API）统一使用 `/api/v1/evaluation` 前缀。应用程序编程接口密钥（API Key）需要 `run_evaluations` 能力或 full-access 权限。查看者（Viewer）可以读取数据，管理员（Admin）可以创建、取消和标注任务，并可以写入数据集版本与人工评分。`DELETE /evaluation/:task_id` 仅接受 Admin 用户登录令牌。

导出接口支持 JavaScript 对象表示法（JavaScript Object Notation，JSON）和逗号分隔值（Comma-Separated Values，CSV）。

## 路由

| 方法 | 路径 | 权限 | 响应数据 |
| --- | --- | --- | --- |
| POST | `/evaluation` | Admin | `EvaluationTask` |
| GET | `/evaluation?task_id=...` | Viewer | `EvaluationDetail` |
| GET | `/evaluation/metrics` | Viewer | `{items: EvaluationMetricDefinition[]}` |
| GET | `/evaluation/tasks` | Viewer | `{items: EvaluationTask[], next_cursor}` |
| PUT | `/evaluation/tasks/:task_id/labels` | Admin | `{task_id, labels}` |
| POST | `/evaluation/comparisons` | Viewer | `EvaluationComparisonResponse` |
| GET | `/evaluation/tasks/:task_id/export?format=json|csv` | Viewer | 下载文件 |
| POST | `/evaluation/:task_id/cancel` | Admin | 当前 `EvaluationTask` |
| DELETE | `/evaluation/:task_id` | Admin 用户令牌 | `204 No Content` |
| GET | `/evaluation/tasks/:task_id/questions` | Viewer | `{items: EvaluationQuestionResultEntity[], next_cursor}` |
| GET | `/evaluation/tasks/:task_id/questions/:sample_index/ratings` | Viewer | `{items: EvaluationHumanRatingRevision[]}` |
| POST | `/evaluation/tasks/:task_id/questions/:sample_index/ratings` | Admin | `EvaluationHumanRatingRevision` |
| POST | `/evaluation/datasets` | Admin | `EvaluationDataset` |
| GET | `/evaluation/datasets` | Viewer | `{items: EvaluationDataset[]}` |
| POST | `/evaluation/datasets/:id/versions` | Admin | `EvaluationDatasetVersion` |
| GET | `/evaluation/datasets/:id/versions` | Viewer | `{items: EvaluationDatasetVersion[]}` |

除下载和 `204` 响应外，接口使用 `{"success":true,"data":...}` 外层结构。

## 创建任务

`POST /evaluation` 的 JSON 请求字段如下：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `dataset_id` | string | 否 | 数据集标识符（Identifier，ID） |
| `dataset_version_id` | string | 否 | 不可变数据集版本 ID |
| `knowledge_base_id` | string | 否 | 源知识库 ID |
| `chat_id` | string | 否 | 对话模型 ID |
| `rerank_id` | string | 否 | 重排序模型 ID |
| `seed` | integer | 否 | 生成随机种子；省略值与显式 `0` 分开处理 |
| `configuration.retrieval` | object | 否 | `vector_threshold`、`keyword_threshold`、`embedding_top_k` |
| `configuration.rerank` | object | 否 | `rerank_top_k`、`rerank_threshold` |
| `configuration.generation` | object | 否 | `temperature`、`top_p`、`top_k`、`max_tokens` |

```bash
curl -X POST 'http://localhost:8080/api/v1/evaluation' \
  -H 'X-API-Key: sk-xxxxx' -H 'Content-Type: application/json' \
  -d '{"dataset_id":"golden","dataset_version_id":"version-1","chat_id":"model-1","seed":0}'
```

模型提供方不支持请求种子时返回 `422`。数据集、版本或源知识库不可见时返回 `404`。

## 任务 DTO

数据传输对象（Data Transfer Object，DTO）`EvaluationTask` 包含 `id`、`tenant_id`、`dataset_id`、`start_time`、`status`、`labels`、`provenance_complete`、`total` 和 `finished`。可选字段包括 `end_time`、`err_msg`、`cancel_requested_at`、`cleanup_errors` 和 `dataset_version_id`。

任务状态值如下：

| 值 | 名称 | 含义 |
| --- | --- | --- |
| 0 | Pending | 等待执行 |
| 1 | Running | 正在执行 |
| 2 | Success | 成功结束 |
| 3 | Failed | 执行失败 |
| 4 | TimedOut | 达到任务期限 |
| 5 | Interrupted | 租约到期且恢复清理完成 |
| 6 | Canceled | 持久化取消请求已完成处理 |

`EvaluationDetail` 在任务字段之外返回 `params`、可选 `metric`、可选 `runtime_metrics`、`experiment` 和 `provenance_complete`。`experiment` 保存数据集、模型、参数、指标计划、代码和运行环境快照。

## 任务列表、标签、比较和导出

`GET /evaluation/tasks` 按 `(start_time DESC, id DESC)` 使用不透明游标分页。查询参数包括：

| 参数 | 说明 |
| --- | --- |
| `status` | 数值状态 |
| `dataset_id` | 数据集 ID |
| `dataset_version_id` | 数据集版本 ID |
| `model_id` | 实验快照中的模型 ID |
| `started_from` | 开始时间下界，含边界 |
| `started_to` | 开始时间上界，不含边界 |
| `label` | 标签交集筛选，可重复传入 |
| `page_size` | 默认 20，最大 100 |
| `cursor` | 上一页返回的不透明游标 |

时间参数使用 RFC 3339（Request for Comments 3339）格式。标签请求为 `{"labels":[...]}`，服务执行全量替换并返回规范化结果。

比较请求包含 2 至 10 个 `task_ids` 和可选 `baseline_task_id`。响应提供运行摘要、冻结参数差异、指标绝对值、相对基线变化、置信区间、延迟分位数和 Token 合计。导出接口要求 `format=json` 或 `format=csv`，只导出终态任务，并应用条数和文件大小上限。

## 逐题结果与人工评分

`GET /evaluation/tasks/:task_id/questions` 按 `sample_index` 升序分页，`page_size` 默认 100、最大 500。逐题记录包含问题和参考答案、ground truth 段落、检索与重排序结果、生成文本、逐样本指标、指标观测、阶段耗时、Token 用量、状态和结果哈希。

人工评分请求字段为：

| 字段 | 约束 |
| --- | --- |
| `rubric_key` | 必填字符串，最长 64 字符 |
| `rubric_version` | 必填字符串，最长 32 字符 |
| `rubric_snapshot` | 必填 JSON 对象 |
| `score` | 必填整数，1 至 5 |
| `comment` | 可选字符串，最长 4000 字符 |

每次写入创建不可变修订。响应包含修订号、评分人、评分量表快照、分数、评论、创建时间和可选 `supersedes_id`。

## 数据集与版本 DTO

`POST /evaluation/datasets` 接受 `name` 和可选 `description`。数据集响应包含 `id`、`scope`、可选 `owner_tenant_id`、`name`、`description`、`current_version_id`、`created_at` 和 `updated_at`。

`POST /evaluation/datasets/:id/versions` 接受以下结构：

```json
{
  "passages": [{"pid":"p-1","content":"段落内容","metadata":{"source":"manual"}}],
  "questions": [{"qid":"q-1","question":"问题文本","answer":"参考答案"}],
  "relevance": [{"qid":"q-1","pid":"p-1","grade":1}]
}
```

版本响应包含 `id`、`dataset_id`、`version_number`、`schema_version`、`artifact_sha256`、`content_sha256`、`manifest`、`passage_count`、`question_count`、`relevance_count` 和 `created_at`。

## 指标目录

`GET /evaluation/metrics` 返回版本化指标目录。每个定义包含 `key`、`version`、`kind`、`description`、`default_config` 和 `config_schema`。任务指标的 `scores` 字段以指标实例 ID 为键，值包含可空 `value`、`status` 和可选 `error_code`。
