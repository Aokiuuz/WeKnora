# M4 实验对比与导出稳定事实

里程碑 4（Milestone 4，M4）的稳定事实如下。

## 1. 权威入口

- 集成分支：`integration/evaluation-m4-comparison-export`。
- M3 基线：`a135f57f8a166e33e3e5ae7a700011661184ef1b`。
- 权威业务仓库：`D:\XINIUNIAO\recovery\20260831-git-recovery\recovered\WeKnora-topic3`。
- PostgreSQL 标签迁移：`000097`。
- SQLite 标签迁移：`000020`。

## 2. 查询与标签

- 任务列表按 `(start_time DESC, id DESC)` 使用不透明键集游标。
- 筛选条件包括状态、数据集 ID、数据集版本 ID、模型 ID、开始时间范围和最多 5 个标签。
- 游标绑定全部筛选条件；跨筛选条件复用返回无效游标。
- 标签规范化为小写，单任务最多 20 个，单标签最多 64 个 8 位统一码转换格式
  （8-bit Unicode Transformation Format，UTF-8）字节。
- 标签替换使用单个数据库事务，不修改任务业务版本和更新时间。

## 3. 对比协议

- 对比接受 2 至 10 个稳定去重任务，默认使用请求中的第一个任务作为基线。
- 服务层用一次租户查询读取任务；缺失、跨租户和应用程序编程接口密钥
  （Application Programming Interface Key，API Key）范围外对象使用 NotFound 语义。
- 参与任务必须为 Success、溯源完整且数据集内容安全哈希算法 256 位
  （Secure Hash Algorithm 256-bit，SHA-256）一致。
- 参数差异只读取六个稳定根：`/dataset`、`/models`、`/configuration/retrieval`、
  `/configuration/rerank`、`/configuration/generation`、`/metric_plan`。
- 指标对比使用全部数值叶子。`key`、`version` 和 `config_sha256` 一致时计算绝对差值和相对差值。
- 基线为零时相对差值为 `null`；指标身份不兼容时全部差值为 `null`。
- 服务端不生成 improved、regressed 或同类方向性判断。

## 4. 导出协议

- Success、Failed、TimedOut、Interrupted 与 Canceled 可以导出；Pending 与 Running 返回 HTTP 409。
- 导出支持 JavaScript 对象表示法（JavaScript Object Notation，JSON）schema version 1 和固定列
  逗号分隔值（Comma-Separated Values，CSV）。
- CSV 使用 `run` 与 `question` 两种记录；用户文本和复杂值均写成 JSON 字面量。
- 构建页大小为 500，问题上限为 50,000，文件上限为 256 MiB。
- 临时文件完整构建并关闭后才开始响应；所有构建错误清理临时文件；客户端慢速读取期间不持有数据库事务。
- Go 软件开发工具包（Software Development Kit，SDK）使用流式响应和 `io.Copy`，不使用 `io.ReadAll`，
  不继承通用 30 秒请求超时。

## 5. 权限与前端

- 查询、问题明细、对比和导出需要 Viewer；标签替换需要 Admin。
- API Key 查询、对比和导出需要 `run_evaluations` 能力，并使用冻结实验清单中的来源知识库 ID 授权。
- 前端入口为 `/platform/evaluations`，包含筛选、运行浏览、详情、问题结果、2 至 10 个运行对比、标签编辑和
  JSON/CSV 下载。
- 浏览器直接呈现服务端参数差异和指标差异，不重算指标。
- 英语、简体中文、俄语和韩语词条保持相同键集合。

## 6. 验证状态

- 根 Go 模块和独立 SDK 模块全量测试通过。
- PostgreSQL 与 SQLite 连续迁移往返通过，版本分别为 97 和 20。
- M4 仓储、服务、处理器、路由、数据库和 SDK 竞态检测通过。
- 根模块和 SDK `go vet` 通过。
- `golangci-lint v2.12.2` 相对 M3 基线为 0 issues。
- 前端 513 项测试、类型检查和生产构建通过。

## 7. M5 边界

- 里程碑 5（Milestone 5，M5）消费 M3 指标数据传输对象和 M4 对比、导出协议。
- M5 负责指标注册适配、统计扩展、性能基线、成本观测和运行负载验证。
- M5 保持 M4 查询筛选、标签事务、对比 schema、导出 schema、权限和文件边界。
