# 评估功能 API

[返回目录](./README.md)

| 方法 | 路径           | 描述                  |
| ---- | -------------- | --------------------- |
| GET  | `/evaluation/` | 获取评估任务结果       |
| POST | `/evaluation/` | 创建评估任务          |

> 注：服务端路由带尾斜杠（Gin 会自动从 `/evaluation` 重定向到 `/evaluation/`），下方示例为方便阅读用了 `/evaluation`。

## GET `/evaluation` - 获取评估任务结果

**参数说明（查询参数）**:

| 字段     | 类型   | 必填 | 说明                                                |
| -------- | ------ | ---- | --------------------------------------------------- |
| task_id  | string | 是   | 从 `POST /evaluation` 返回的任务 ID                  |

通用唯一标识符（Universally Unique Identifier，UUID）用于区分同一毫秒内创建的任务。任务 ID 格式为 `evaluation_<tenantID>_<Unix 毫秒时间戳>_<8 位 UUID 片段>_<datasetID>`。

**请求**:

```bash
curl --location 'http://localhost:8080/api/v1/evaluation?task_id=evaluation_1_1787880000000_a1b2c3d4_default' \
--header 'X-API-Key: sk-xxxxx' \
--header 'Content-Type: application/json'
```

**响应**:

```json
{
    "data": {
        "task": {
            "id": "evaluation_1_1787880000000_a1b2c3d4_default",
            "tenant_id": 1,
            "dataset_id": "default",
            "start_time": "2025-08-12T14:54:26.221804768+08:00",
            "status": 2,
            "total": 1,
            "finished": 1
        },
        "params": {
            "session_id": "",
            "knowledge_base_id": "2ef57434-8c8d-4442-b967-2f7fc578a2fc",
            "vector_threshold": 0.5,
            "keyword_threshold": 0.3,
            "embedding_top_k": 10,
            "vector_database": "",
            "rerank_model_id": "b30171a1-787b-426e-a293-735cd5ac16c0",
            "rerank_top_k": 5,
            "rerank_threshold": 0.7,
            "chat_model_id": "8aea788c-bb30-4898-809e-e40c14ffb48c",
            "summary_config": {
                "max_tokens": 0,
                "repeat_penalty": 1,
                "top_k": 0,
                "top_p": 0,
                "frequency_penalty": 0,
                "presence_penalty": 0,
                "prompt": "这是用户和助手之间的对话。",
                "context_template": "你是一个专业的智能信息检索助手",
                "no_match_prefix": "<think>\n</think>\nNO_MATCH",
                "temperature": 0.3,
                "seed": 0,
                "max_completion_tokens": 2048
            },
            "fallback_strategy": "",
            "fallback_response": "抱歉，我无法回答这个问题。"
        },
        "metric": {
            "retrieval_metrics": {
                "precision": 0,
                "recall": 0,
                "ndcg3": 0,
                "ndcg10": 0,
                "mrr": 0,
                "map": 0
            },
            "generation_metrics": {
                "bleu1": 0.037656734016532384,
                "bleu2": 0.04067392145167686,
                "bleu4": 0.048963321289052536,
                "rouge1": 0,
                "rouge2": 0,
                "rougel": 0
            }
        }
    },
    "success": true
}
```

检索指标按评估管道的结果顺序计算。无法归属到本次临时知识的结果和重复数据集段落标识（passage ID，PID）结果保留排名位置，并按未命中计分。

## POST `/evaluation` - 创建评估任务

**参数说明（请求体）**:

| 字段              | 类型   | 必填 | 说明 |
| ----------------- | ------ | ---- | ---- |
| dataset_id        | string | 否   | 空值使用 `default`；当前固定加载 `dataset/samples/` |
| knowledge_base_id | string | 否   | 空值使用默认模型创建临时评估知识库；提供时复制该知识库的模型配置创建临时知识库 |
| chat_id           | string | 否   | 空值自动选择可用的知识问答（KnowledgeQA）模型；没有可用模型时任务创建失败 |
| rerank_id         | string | 否   | 空值自动选择可用的重排序（Rerank）模型；没有可用模型时跳过重排 |

**请求**:

```bash
curl --location 'http://localhost:8080/api/v1/evaluation' \
--header 'X-API-Key: sk-xxxxx' \
--header 'Content-Type: application/json' \
--data '{
    "dataset_id": "default",
    "knowledge_base_id": "kb-00000001",
    "chat_id": "8aea788c-bb30-4898-809e-e40c14ffb48c",
    "rerank_id": "b30171a1-787b-426e-a293-735cd5ac16c0"
}'
```

**响应**:

```json
{
    "data": {
        "task": {
            "id": "evaluation_1_1787880000000_a1b2c3d4_default",
            "tenant_id": 1,
            "dataset_id": "default",
            "start_time": "2025-08-12T14:54:26.221804768+08:00",
            "status": 1
        },
        "params": {
            "session_id": "",
            "knowledge_base_id": "2ef57434-8c8d-4442-b967-2f7fc578a2fc",
            "vector_threshold": 0.5,
            "keyword_threshold": 0.3,
            "embedding_top_k": 10,
            "vector_database": "",
            "rerank_model_id": "b30171a1-787b-426e-a293-735cd5ac16c0",
            "rerank_top_k": 5,
            "rerank_threshold": 0.7,
            "chat_model_id": "8aea788c-bb30-4898-809e-e40c14ffb48c",
            "summary_config": {
                "max_tokens": 0,
                "repeat_penalty": 1,
                "top_k": 0,
                "top_p": 0,
                "frequency_penalty": 0,
                "presence_penalty": 0,
                "prompt": "这是用户和助手之间的对话。",
                "context_template": "你是一个专业的智能信息检索助手，xxx",
                "no_match_prefix": "<think>\n</think>\nNO_MATCH",
                "temperature": 0.3,
                "seed": 0,
                "max_completion_tokens": 2048
            },
            "fallback_strategy": "",
            "fallback_response": "抱歉，我无法回答这个问题。"
        }
    },
    "success": true
}
```

## M5 指标、统计与人工评分入口

里程碑 5（Milestone 5，M5）增加版本化指标目录、模型统计、价格版本和人工评分修订。下表中的 Viewer 表示查看者
角色，Admin 表示管理员角色；应用程序编程接口密钥（Application Programming Interface Key，API Key）读取指标目录
时需要 `run_evaluations` 能力。

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/evaluation/metrics` | Viewer / API Key | 返回指标 key、version、类别、默认配置和配置 schema |
| GET | `/api/v1/models/:id/usage?from=&to=` | Viewer | 返回单模型调用、Token、费用、缓存和延迟统计 |
| GET | `/api/v1/models/usage?from=&to=&model_ids=` | Viewer | 返回一个或多个模型的时间区间统计 |
| GET | `/api/v1/models/:id/pricing` | Viewer | 返回按生效时间倒序排列的价格版本 |
| PUT | `/api/v1/models/:id/pricing` | Admin | 创建一个有效区间不重叠的价格版本 |
| GET | `/api/v1/evaluation/tasks/:task_id/questions/:sample_index/ratings` | Viewer | 返回人工评分修订 |
| POST | `/api/v1/evaluation/tasks/:task_id/questions/:sample_index/ratings` | Admin | 追加人工评分修订 |

模型统计使用协调世界时（Coordinated Universal Time，UTC）半开区间 `[from, to)`。省略时间时返回最近 30 天，
最长区间为 366 天。响应中的 `latency` 包含 p50、p95、p99 和可报告调用数。`provider_cache` 以厂商报告的 read 与
miss Token 为分母；`application_cache` 以 hit 与 miss 项为分母，并独立返回 bypass 查询数。

价格单位为每百万输入或输出 Token 对应的整数微货币。调用开始时冻结有效价格；缺少价格或用量的调用返回空费用并计入
`unpriced_calls`。人工评分请求包含 rubric key、rubric version、rubric snapshot、score 和 comment；服务端分配修订号并
连接同一 rubric 的 `supersedes_id`。
