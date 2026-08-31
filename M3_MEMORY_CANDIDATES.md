# M3 可复现实验稳定事实

里程碑 3（Milestone 3，M3）的稳定事实如下。

## 1. 权威入口

- 集成分支：`integration/evaluation-m3-reproducible-experiments`。
- M2 冻结基线：`a05eb06971dcc6b8680bbf3fe6996941518f5a09`。
- 权威业务仓库：`D:\XINIUNIAO\recovery\20260831-git-recovery\recovered\WeKnora-topic3`。
- PostgreSQL 迁移号段：`000094` 至 `000096`。
- SQLite 迁移号段：`000017` 至 `000019`。

## 2. 数据集与实验清单

- 规范化数据库记录是评测输入的权威来源；运行时使用任务冻结的 `dataset_version_id`。
- 数据集内容安全哈希算法 256 位（Secure Hash Algorithm 256-bit，SHA-256）使用 schema version 1：
  passage 按段落标识（Passage ID，PID）、question 按问题标识（Question ID，QID）、relevance 按 QID/PID 排序，
  采用禁用超文本标记语言（HyperText Markup Language，HTML）转义的 JavaScript 对象表示法行
  （JavaScript Object Notation Lines，JSON Lines），保留原始 8 位统一码转换格式
  （8-bit Unicode Transformation Format，UTF-8）内容。
- 内置 `default` 数据集由五个嵌入式 Parquet 文件注册，制品 SHA-256 为
  `d598d48018f1da0705920f4432b68a9309c391adbe1403bb1278771b22fe923f`。
- 新任务保存 `dataset_version_id`、`dataset_content_sha256`、`experiment_snapshot` 和
  `experiment_sha256`。详情使用 `provenance_complete` 表示来源完整性。
- 实验清单冻结数据集、来源知识库、四个模型角色、解析后的配置、指标计划、代码和运行环境摘要。
- 模型配置指纹排除密钥、自定义授权头、URL 凭据和敏感扩展字段；运行阶段重新校验指纹。

## 3. 指标与逐问题事实

- M3 唯一定义 `EvaluationMetricPlanSnapshot`、`EvaluationMetricSpecSnapshot` 和
  `EvaluationMetricObservationSnapshot` 的 JSON 与规范哈希语义。
- 这些类型是数据传输对象（Data Transfer Object，DTO）。指标实例 ID 为 `key@version#config_sha256`。
  里程碑 4（Milestone 4，M4）直接消费，里程碑 5（Milestone 5，M5）通过适配器生成。
- `evaluation_question_results` 主键为 `(tenant_id, task_id, sample_index)`；复合外键对任务物理删除执行级联。
- 问题发布事务同时执行逐问题插入、进度递增、聚合指标更新和任务版本递增。
- 问题发布条件包含租户、任务、owner、Running、version、有效租约和无取消请求。
- 相同结果哈希重试幂等，不同结果哈希冲突；取消与发布竞争由同一数据库事务裁决。
- 未知来源和重复来源保存 `pid=-1`，原始搜索与重排顺序保持不变。

## 4. 随机种子、权限与时间

- seed 指针区分未提供与显式 `seed=0`。OpenAI-compatible 与 Ollama 真实下发 seed；Anthropic 返回类型化不支持错误。
- 受知识库范围限制的应用程序编程接口密钥（Application Programming Interface Key，API Key）使用实验清单中的
  `source_knowledge_base_id`，不使用临时评测知识库 ID。
- 来源不完整或超出允许范围时，详情、取消、列表和逐问题查询使用 NotFound 语义。
- 数据库读取的任务、数据集与逐问题时间在应用层统一为协调世界时（Coordinated Universal Time，UTC）。

## 5. 回归基线

- Golden fixture：`dataset/golden/v1/dataset.json` 与 `dataset/golden/v1/expected.json`。
- Golden 内容 SHA-256：`d8deca2c2ab7b77123130788c9082d0998ecbf76c1d8556952703045cc2556e1`。
- 聚合期望：precision `0.5`、recall `0.75`、归一化折损累计增益
  （Normalized Discounted Cumulative Gain，NDCG）@3 与 NDCG@10 `0.8065735963827292`、
  平均倒数排名（Mean Reciprocal Rank，MRR）`1.0`、平均准确率均值（Mean Average Precision，MAP）`1.0`、
  双语评估替补（Bilingual Evaluation Understudy，BLEU）-1/2/4 `0.5`、面向召回的摘要评估
  （Recall-Oriented Understudy for Gisting Evaluation，ROUGE）-1/2/L `0.5`。
- 浮点容差为 `1e-9`；固定输入测试连续 100 次一致，不访问真实模型或网络。

## 6. M4 冻结边界

- M4 查询、对比和导出直接读取 M3 的实验清单、指标快照和逐问题结果。
- M4 不重建指标 DTO、规范序列化器、数据集哈希、模型指纹或来源知识库授权规则。
- 列表排序继续使用 `(start_time DESC, id DESC)`；逐问题排序继续使用 `sample_index ASC`。
- M4 标签写入不修改 M2 任务业务版本或更新时间。
