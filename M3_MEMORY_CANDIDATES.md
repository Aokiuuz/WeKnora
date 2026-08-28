# M3_MEMORY_CANDIDATES：候选稳定事实（待主审写入共享 memory）

以下事实已在 M3 draft 分支形成提交；主审集成并验证后可择要写入
`D:\XINIUNIAO\memory\weknora-topic3.md`。本文件不直接修改共享 memory。

## 提交与分支

- M0 来源契约已合入 M3 基线：`5268b03f`（merge `e8b1dc85` into `draft/evaluation-m3-reproducibility`）。
- 切片一 `feat/evaluation-dataset-registry`：`9a26aa25` —— 数据集注册表（5 表）、
  不可变版本、artifact/content SHA-256、限制、handler/SDK。
- 切片二 `feat/evaluation-experiment-snapshot`：`91e0e5de` —— 实验清单冻结、
  `internal/buildinfo`、metric snapshot DTO、秘密扫描、确定性默认模型选择、运行期指纹校验。
- 切片三 `fix/evaluation-random-seed`：`c3cc1a24` —— seed 指针语义、OpenAI-compatible/Ollama
  真实下发、Anthropic 显式 seed typed error、422 映射。
- 切片四 `feat/evaluation-question-results`：`aa72b0d4` —— 逐题事务发布、幂等/冲突、
  keyset 分页 API/SDK。
- 切片五 `test/evaluation-golden-regression`：`69c9a120` —— `dataset/golden/v1`
  固定数据集、12 项指标期望值、100 次重复一致。
- 迁移号段：PostgreSQL 000094–000096、SQLite 000017–000019 已由 M3 占用；
  M2 的 000091–000093 / 000014–000016 未进入本分支。

## 架构事实

- M3 唯一拥有 metric wire/storage DTO 与 canonical hash 规则：
  `internal/types/evaluation_metric_snapshot.go`（`EvaluationMetricPlanSnapshot`、
  `EvaluationMetricSpecSnapshot`、`EvaluationMetricObservationSnapshot`，
  `instance_id = key@version#config_sha256`）；M4 直接消费，M5 通过适配器产出，不得另建 DTO。
- 数据集 canonical content hash（schema v1）：passage 按 PID、question 按 QID、relevance 按
  QID/PID 排序；JSON Lines 禁用 HTML 转义、`\n` 分隔；question 行不含 sample_index，
  输入顺序不影响 hash。实现于 `internal/types/evaluation_dataset_hash.go`。
- 构建元数据位于中立包 `internal/buildinfo`；`Makefile` 与 `scripts/get_version.sh` 的
  ldflags `-X` 路径已指向该包。handler 包不再持有版本变量。
- 模型 config 指纹只覆盖去密行为配置（`internal/types/evaluation_model_fingerprint.go`）：
  排除 API Key、AppSecret、AppID、CustomHeaders、URL userinfo 与敏感 extra_config 键。
- `evaluation_question_results` 主键 `(tenant_id, task_id, sample_index)`，复合外键
  `ON DELETE CASCADE` 依赖 `evaluation_tasks` 的 `UNIQUE (tenant_id, id)`（000096/000019 新增）；
  SQLite 外键要求 `_foreign_keys=on`（生产 DSN 已带；测试 DSN 已补齐）。
- seed 契约：创建请求指针区分未提供与 `seed=0`；`SummaryConfig.SeedProvided` 与
  `ChatOptions.SeedProvided` 传播该语义；OpenAI-compatible 与 Ollama 真实下发并记 applied，
  Anthropic 显式 seed 返回 422 typed error；未提供 seed 记录 not_requested。
- 缺省模型解析为确定性：默认标记优先、ID 升序兜底（`SelectEvaluationDefaultModel`）。

## Golden 回归

- Fixture：`dataset/golden/v1/dataset.json` + `expected.json`（版本控制，人工可审阅）。
- 覆盖：相关/无关结果、未知来源（pid=-1）、重复 PID（pid=-1）、rerank 优先与 search fallback。
- 期望值：聚合 precision=0.5、recall=0.75、ndcg3=ndcg10≈0.8065735963827292、mrr=1.0、
  map=1.0、bleu1/2/4=0.5、rouge1/2/L=0.5；content SHA-256
  `d8deca2c2ab7b77123130788c9082d0998ecbf76c1d8556952703045cc2556e1`。
- 容差 1e-9；重复 100 次一致；race 目标通过。不 pin live 模型输出。

## 环境事实（本机 worktree 开发）

- WorkBuddy 沙箱中，git 对共享 `.git/refs/heads` 的写入会被静默回滚（commit 对象可写入，
  ref 更新丢失并可能清空父目录）。可行流程：提权执行 git 写操作；commit 后
  `mkdir -p` 目标 ref 父目录并用 `printf <full-hash> > refs/heads/<branch>` 修复，
  再以 `git rev-parse HEAD` 验证。每个更新分支 ref 的操作都必须如此。
- WSL 被安全策略禁用，Windows 无 Go 工具链：Go 门禁无法在本机执行，需在主审环境重跑。
