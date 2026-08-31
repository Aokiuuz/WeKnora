# M3_INTEGRATION：待主审接线的 M2 依赖点

本文件列出 M3 草稿（`draft/evaluation-m3-reproducibility`）中所有依赖 M2c–M2e 的窄接口、
CAS 条件、迁移外键和测试接线点。本 checkout 不包含 M2c–M2e（`cancel_requested_at`、
列表/删除/retention 迁移 000091–000093 / 000014–000016 未进入本分支），以下各项均未验证，
不得在集成前描述为已通过。

## 1. 连续迁移链

- M3 使用预留号段：PostgreSQL 000094–000096、SQLite 000017–000019。本分支 SQLite 迁移从
  000013 直接跳到 000017（golang-migrate 允许跳号），中间 000014–000016 由 M2 窗口负责。
- 主审集成 M2d/M2e 后必须验证完整连续迁移（000013→000019 与 000090→000096）的 up/down。
- PostgreSQL 000096 在 `evaluation_tasks` 上新增 `UNIQUE (tenant_id, id)` 约束供复合外键引用；
  SQLite 000019 以唯一索引实现同等约束。若 M2d 迁移也修改 `evaluation_tasks`，主审需检查约束冲突。
- `migration_sqlite_versioned_schema_test.go` 的 `expectedSQLiteMigrationVersion` 当前为 19；
  M2 的 000014–000016 进入后保持 19 不变（19 已含 M2 区间之外的 M3 号段），但测试需覆盖 M2 新增表/列。

## 2. 取消 CAS（M2d）

- 窄接口：`interfaces.EvaluationTaskCancellationChecker`
  （`internal/types/interfaces/evaluation_question_result.go`）。
- 当前接线：`internal/container/container.go` 中 `NewEvaluationQuestionResultRepository(db, nil)`，
  nil 表示本 draft 的 schema 尚无 `cancel_requested_at` 列。
- 集成动作：M2d 落地后用 `cancel_requested_at IS NULL` 谓词实现 checker 并替换 nil；
  或在 `PublishQuestionResult` 的事务 UPDATE 中直接加列条件（二选一，主审决定）。
- 测试接线：`TestEvaluationQuestionResultRejectsCanceledTask` 当前用 fake checker 证明取消拒绝路径；
  集成后应增加基于真实 `cancel_requested_at` 列的用例。
- 终态选择优先级（Canceled vs TimedOut vs Failed）与 `cancel_requested_at` 真值表由 M2d 拥有，M3 未实现。

## 3. Retention 级联与软删除（M2e）

- `evaluation_question_results` 通过复合外键 `ON DELETE CASCADE` 绑定 `evaluation_tasks(tenant_id, id)`；
  M2e 保留清理物理删除任务行时逐题行级联删除。SQLite 依赖生产 DSN 的 `_foreign_keys=on`
  （container.go:705 已配置）；测试 DSN 已在 `openSQLiteDB` 与 `setupEvaluationTaskRepositoryTestDB` 补齐。
- `evaluation_question_results.deleted_at` 已预留；`ListQuestionResults` 已过滤 `deleted_at IS NULL`，
  分页索引为部分索引。M2e 任务软删除时是否同事务软删逐题行由主审决定（当前软删任务不影响逐题查询，
  因为逐题 API 以 tenant+task 为边界且任务本身已被查询层过滤）。
- 活动任务（Pending/Running）永不进入保留清理由 M2e 保证，M3 未实现。

## 4. API Key KB allow-list 授权

- 实验清单的 `source_knowledge_base_id` 已冻结进 `evaluation_tasks.experiment_snapshot`
  （原始创建请求值，null 表示无来源 KB），并参与 `experiment_sha256`。
- M3 未实现授权中间件变更。集成动作：M2d/M4 的 API Key 授权路径从实验清单读取
  `source_knowledge_base_id` 作为 allow-list 唯一输入；带 KB allow-list 的 API Key 访问
  provenance 不完整（`experiment_snapshot IS NULL`）或 source 为空的旧任务时默认拒绝并返回 404。
  不得使用 `temporary_kb_id`。

## 5. 容器与装配

- `NewEvaluationService` 新增第 8、9 参数：`datasetRegistry`、`questionResultRepository`；
  container 已 Provide `NewEvaluationDatasetRegistryService` 与
  `NewEvaluationQuestionResultRepository`，dig 图完整。M2 若也修改构造函数，主审合并参数列表。
- `RegisterEvaluationRoutes` 现接收 3 个 handler（evaluation、dataset、question）；
  `router.go` RouterParams 与 `router_api_key_capabilities_test.go` 已同步。

## 6. buildinfo 与版本注入

- 构建元数据移至中立包 `internal/buildinfo`（linker 变量 + `debug.ReadBuildInfo` 回退）。
- `Makefile` 与 `scripts/get_version.sh` 的 `-X` 注入路径已从 `internal/handler.*` 改为
  `internal/buildinfo.*`。主审需验证 Docker 构建路径（get_version.sh 的 docker/build-arg 模式）
  与实际 Dockerfile 的 ldflags 引用一致。
- `cmd/desktop/update.go`、`internal/router/router.go`、`internal/router/deployment_capabilities.go`
  已改引用 `buildinfo.*`；handler 包不再持有版本变量。

## 7. 内置数据集注册

- 受控注册流程 `EvaluationDatasetRegistryService.RegisterBuiltinDataset` 已实现
  （artifact SHA-256 pin 校验、内容幂等、系统作用域）。
- 未接线：`dataset/samples` 五个 Parquet → 结构化注册输入的转换器、pin 的
  artifact SHA-256（需运行环境计算）、启动时自动注册流程。主审在有 Go 环境时生成
  manifest 并接入启动序列；`GetDatasetByID` 当前仍读取固定 Parquet 文件，未切换到 registry 读取
  （切换属于后续切片，需保持评测执行与 registry 版本绑定一致）。

## 8. 测试与门禁未验证项

- 全部 Go 门禁（gofumpt、gofmt、go test、race、go vet、golangci-lint、CGO build、Compose config）
  因本机 WSL 被安全策略禁用且 Windows 无 Go 工具链而未执行。详见
  `docs/reports/2026-08-28-m3-draft-acceptance.md` 的命令与未验证项清单。
- `TestEvaluationQuestionResultPostgresContract`、`TestEvaluationDatasetRepositoryPostgresContract`
  与 000095 的 PG 断言需要 `TEST_POSTGRES_DSN`；未运行。
