# M5 指标、模型观测、缓存与性能稳定事实

## 1. 权威入口

- 集成分支：`integration/evaluation-m5-observability-performance`。
- M4 基线：`f5e3d11a7f502f1ec07302d1844a20f22f5b297c`。
- 权威仓库：`D:\XINIUNIAO\recovery\20260831-git-recovery\recovered\WeKnora-topic3`。
- PostgreSQL 迁移范围：`000098` 至 `000102`。
- SQLite 迁移范围：`000021` 至 `000025`。

## 2. 指标与运行事实

- 指标注册键为 `(key, version)`；重复注册失败。
- 指标实例 ID 为 `key@version#config_sha256`，配置与计划复用 M3 规范序列化协议。
- 默认计划包含 12 个兼容指标实例；插件观测进入动态 `scores`。
- 运行指标 schema version 1 保存阶段耗时、样本状态分母、失败分数和 Token 报告状态。
- 普通分数使用固定 seed 的 bootstrap 区间，二元成功率使用 Wilson 区间。
- 有效样本少于 2 个时置信区间状态为 `insufficient_sample`。

## 3. 模型账本与价格

- Chat、Embedding、Rerank、视觉语言模型和自动语音识别通过集中模型工厂安装观测装饰器。
- 每个厂商调用对应一条 `model_call_records`；流式终态使用 `sync.Once` 收敛。
- 评测调用采用严格记账，普通交互采用尽力记账。
- 账本不保存提示词、业务正文、媒体、密钥和授权头。
- 价格版本按有效区间保存且禁止重叠；调用开始时冻结价格快照。
- 费用使用整数微货币、大整数中间值和 half-up 舍入；缺价格或用量时费用为 `null`。

## 4. Embedding 缓存

- 缓存键包含租户、模型 ID、模型指纹、请求选项 SHA-256 和原始 UTF-8 文本 SHA-256。
- 向量使用 little-endian float32 二进制，PostgreSQL 为 BYTEA，SQLite 为 BLOB。
- 批处理执行去重、批量读取、缺失项请求、完整校验和原顺序恢复。
- 缓存故障采用 fail-open，并以 bypass 查询记录区分。
- `embedding_cache_lookup_records` 保存 requested、unique、hit、miss、bypass、status 和 duration。
- 查询记录不保存原始文本、文本哈希和向量。
- 默认有效期 30 天，清理周期 24 小时，每批 500 行，每轮 30 秒。

## 5. 统计、界面与 API

- 模型统计返回调用状态、Token、费用、未定价数、p50/p95/p99 延迟和两类缓存口径。
- 厂商缓存分母为可报告的 read 与 miss Token；应用缓存分母为 hit 与 miss 项。
- 应用缓存统计独立返回 bypass 查询数、请求项数、去重项数和平均查询耗时。
- 价格 API 为 `GET/PUT /api/v1/models/:id/pricing`。
- 模型统计抽屉提供 7、30、90 天范围；管理员可写价格，查看者可读统计。
- 人工评分按照任务、样本和 rubric 追加修订，`supersedes_id` 连接修订链。

## 6. 回归、性能与环境条件

- `make evaluation-reproduce` 生成权威 JSON 和派生 Markdown，退化阈值返回非零状态。
- 回归 CI 支持拉取请求、定时和手动触发，不访问外部模型。
- 性能矩阵为 10/100/1,000 样本、1/4/8 并发、冷/热缓存和每组 5 次重复。
- 相同语料热缓存索引构建的厂商输入项为 0。
- Wiki live 厂商缓存结果在无可报告 provider 环境中为 `unavailable`。
- 确定性适配器验证 Wiki 厂商缓存 reported、hit、miss 和 bypass 统计链。
- 语义裁判与收费模型的 live 实测依赖模型、凭据和费用预算。
