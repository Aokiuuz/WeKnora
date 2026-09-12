# 课题三候选版本外部独立验证（2026-09-12）

本报告记录外部实施 agent 在独立克隆中对核准集成基线 `25cf5a95d88607b6ff6a0d92182301f90d45051d` 的无凭据工程验证，以及两项已确认局部修复的验证结果。验证不连接任何既有服务、数据库或账号，供应商为本地回环夹具，付费请求数为 0。本报告对应修复分支 `fix/topic3-final-cross-review-20260912`。

## 环境身份

| 项 | 值 |
| --- | --- |
| 操作系统 | WSL Ubuntu 22.04（Linux，x86-64） |
| Go | go1.26.0（CI 使用 go1.26.7，金色门禁结果逐位一致） |
| GCC | 11.4.0（CGO_ENABLED=1，sqlite_fts5 标签） |
| Node / npm | v24.19.0 / 11.17.0（lockfile 全新安装 380 包，非跨系统复用） |
| Python | 3.10.12 |
| Git | 2.34.1 |
| 克隆来源 | 公开成果分支 tip `0e3fce68` + 增量 bundle（SHA-256 `3c44242f…be14`），导入后 tip 恰为 25cf5a95，`0e3fce68` 与 `8dd3f058` 均为祖先，工作树干净 |

## 基线验证（E01，基线提交 25cf5a95）

| 检查 | 命令/方式 | 退出码 | 结果 |
| --- | --- | --- | --- |
| 金色回归门禁 | `make evaluation-reproduce` | 0 | 13 项冻结指标与基线逐位一致（recall=0.75 等） |
| 指标退化负例 | `evaluation-reproduce --metric-overrides evaluation/regression/testdata/degraded.json` | **1**（预期失败） | 指出 `retrieval.recall baseline=0.75 current=0.5 absolute_delta=0.25 threshold=0` |
| 冻结公开数据校验 | `prepare-public-evaluation.py --check` | 0 | 通过 |
| 公开数据完整性测试 | `prepare-public-evaluation-test.py` | 0 | 12 项通过 |
| 产品版本一致性 | `check-product-version.py` | 0 | 0.8.0，14 个表面无错误 |
| HTTP 端到端冒烟 | `evaluation-rag-http-smoke.py`（本地构建 sqlite_fts5 服务端） | 0 | 冷轮 25 嵌入+20 对话、热轮 0 嵌入+20 对话；API/JSON/CSV/DB 四口径一致；账本逐条对账通过；`paid_provider_requests=0` |
| 故障恢复冒烟 | `evaluation-fault-http-smoke.py` | 0 | 取消、重复取消、进程强杀后租约自动恢复均通过（首跑因输出目录残留报 FileExistsError，清理后复跑通过，两处记录均保留） |
| Go 核心包测试 | `go test -tags sqlite_fts5`（modelcache/modelobs/models/call/models/chat/types/modelstats/evaluation/*） | 0 | 10 个包全部 ok |
| 预算边界测试 | `test_evaluation_openrouter_budget.py` | 0 | 通过 |
| 读者组件测试 | `test_evaluation_expanded_reader.py` | 0 | 通过 |
| 前端定向测试 | `tsx --test`（工作台+用量抽屉） | 0 | 19/19 通过 |
| 前端类型检查 | `npm run type-check` | 0 | 通过 |
| 前端完整套件 | `npm test` | 0 | 675/675 通过 |
| 前端生产构建 | `npm run build` | 0 | 通过（37.49s，dist/index.html 生成） |
| 增量静态检查 | `golangci-lint run --new-from-rev`（v2.12.2，仓库固定版本） | 候选范围 0 / 集成基点范围 1 | 候选改动（25cf→修复）0 问题；集成基点 0062f2f1→候选 26 项全部为解析命令与云适配器的历史遗留（长行/errcheck/gofumpt/unused-parameter），按方案不做批量重构；全仓历史 5,923 项事实保留 |

未执行项：PostgreSQL 契约测试（执行环境无 Docker CLI，无法起一次性容器；sqlite 路径的仓储套件 132s 全套另在原审核中通过）；CI 大体积产物逐字节核验（历史遗留，元数据级核验状态不变）；浏览器 12 项交互（需真实浏览器与账号，沿用 `db56f0d9` 的历史记录，不复称）。

## 真实检索退化门禁验证（E01 补充，A2 的生产路径证据）

针对"金色门禁仅重放录制输出"的覆盖疑虑，本轮用可恢复补丁直接退化生产检索路径：`fuseOrDeduplicate` 只保留融合结果末位（补丁文件 `retrieval-degradation.patch`，1,137 字节，单文件，验证后已 `git checkout` 恢复）。

| 阶段 | 退出码 | 结果 |
| --- | --- | --- |
| 退化二进制构建 | 0 | SHA-256 `f14a6e0d…0339` |
| 退化版 HTTP 冒烟 | **1** | `AssertionError: Round 1 retrieval regression: recall=0 below frozen minimum 0.741666666666667`——真实流水线退化被阻断且明确指出退化指标 |
| 恢复后重新构建 | 0 | 源码恢复干净（0 个修改文件） |
| 恢复版 HTTP 冒烟 | 0 | 冷 25 嵌入/热 0 嵌入、每轮 20 对话、0 付费，全部断言通过 |

该实验不改变冻结数据、指标或阈值；退化只存在于临时二进制与已恢复的补丁中。

## 局部修复验证（E03、E04）

### E03 嵌入缓存写失败可观测性（CXR-010/037）

- 改动：`internal/modelcache/cache.go` 两处 `_ =` 丢弃错误改为 `logger.Warnf` 脱敏告警（仅租户 ID、模型 ID、条目数/状态与存储错误；不含原文、向量、凭据）。最佳努力语义不变：向量照常返回，命中统计仍按既有记录口径。
- 验证：`TestEmbeddingCachePersistFailuresLogSanitizedWarningAndStayFailOpen` 注入 Put/Record 两类写失败，断言向量正常返回、两条告警出现、日志不含源文本、失败事件未落库；`go test ./internal/modelcache ./internal/modelobs` 退出码 0。

### E04 筛选后旧详情一致性（CXR-032）

- 改动：`EvaluationWorkbench.vue` 的 `applyFilters` 在应用新筛选时使已打开详情失效（清空 detail/activeTaskId/questions 并经请求门隔离迟到响应）。复用既有请求序列门，未引入第二套状态管理。CXR-030 所述数值成功率缺陷经复核不成立（字段为置信区间对象），未做模板改动。
- 验证：新增回归用例覆盖"选中任务→筛选无结果→详情清空"与"旧详情响应迟到不恢复"；定向 20/20、全套件 676/676、type-check 与生产构建退出码均为 0。

## 证据身份

- 候选基线：`25cf5a95d88607b6ff6a0d92182301f90d45051d`
- 修复后头提交：见 PR 顶部提交（本文件随其可匿名取得）
- 退化补丁与验证日志：`artifacts/external-validation/`（执行环境本地；本文件为脱敏摘要）
- 历史证据（扩展实测、M4、浏览器、远端门禁）各绑定其原提交，见 `docs/acceptance-demo.md` 的证据版本表；本报告不将历史结果标记为本候选执行。

## 离线失败类型分析（E10，规则分析）

仅使用已公开冻结输出 `docs/reports/expanded-acceptance/reader-scores.json`（1,200 份，原文未改），按互斥规则分类；分母每组 200。本表是规则统计，不代替人工语义判断；正式集已观察，不作未见集声明。

| 数据集 | 模型 | 完全匹配 | 部分重合 F1≥0.5 | 低重合 F1<0.5 | 可答误拒 | 无答案误答 | 无答案正确拒答 | 格式无效 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| CMRC | DeepSeek V4 Flash | 137 | 55 | 6 | 1 | — | — | 1 |
| CMRC | Kimi K2.5 | 145 | 48 | 6 | 0 | — | — | 1 |
| HotpotQA | DeepSeek V4 Flash | 113 | 21 | 38 | 28 | — | — | 0 |
| HotpotQA | Kimi K2.5 | 127 | 26 | 23 | 20 | — | — | 4 |
| SQuAD | DeepSeek V4 Flash | 68 | 16 | 12 | 4 | 57 | 43 | 0 |
| SQuAD | Kimi K2.5 | 71 | 17 | 10 | 2 | 71 | 29 | 0 |

结论：除已知"SQuAD 无答案误答"弱项（57/71 条）外，规则分析还发现 HotpotQA 存在"可答误拒"（两模型 28/20 条，占各自 14%/10%），多跳证据链断裂是合理假设但需人工核验样例确认；CMRC 两模型质量最高且相近。无答案误答与可答误拒是两个不同方向的失败，优化提示词时不能只看单一拒答率。

## 条件项复现与决策（E06、E07）

### E06 延迟分位数规模验证（CXR-012/042）

方法：临时合成工具（固定种子 20260912，验证后已删除）在独立内存 SQLite 构造 1 万/10 万/100 万条 `model_call_records`（5 个模型，时长 100–30,100ms 均匀随机），测量 `QueryModelUsage` 的真实耗时与分配。

| 行数 | 查询耗时 | 查询增量分配 | 常驻分配 |
| ---: | ---: | ---: | ---: |
| 10,000 | 32 ms | 2.4 MB | 3.1 MB |
| 100,000 | 233 ms | 26.4 MB | 13.4 MB |
| 1,000,000 | 2,402 ms | 268.8 MB | 67.3 MB |

决策：**复现成立（线性增长可测），不修改代码**。理由：接口已有 366 天窗口上限；100 万行级调用对应 2.4 秒的管理端分析查询在当前产品语境不构成故障；PostgreSQL `PERCENTILE_CONT` 与现有线性插值的跨库数值一致性风险（方案自评"高"）大于收益。保留精确实现与本测量记录；临时工具未入库。

### E07 条件改进五项决策表

| 子项 | 关联 | 复现结论 | 处置 |
| --- | --- | --- | --- |
| 模型缺名回退 | CXR-031 | 成立（模型不在当前列表时标题显示原始 ID） | 已修：保留 ID 并追加本地化"名称不可用"标记（四语言键同步）；回归测试更新 |
| EvalDataset 双入口 | CXR-034 | 部分成立（有租约认领与 owner 检查，无独立心跳；全仓调用图仅测试调用） | 已加边界注释，标明测试辅助与生产入口 `runEvaluation` 的关系；不改协议 |
| 稀疏 PID 空段 | CXR-035 | 边缘成立：仅无实验清单的遗留路径可达；自带样例 id 为 1..4，回退数组在 index 0 产生 1 个空段 | 不修改：压缩/重排会破坏 PID 与检索证据的索引映射；风险大于收益；记录在案 |
| CSV 状态整数 | CXR-039 | 成立（`run_status` 为整数） | 不改列：在 `docs/acceptance-demo.md` 补状态映射表，保持导出契约与 schema 版本不变 |
| 预算中继共享 round | CXR-044 | 未复现：唯一已知并发调用方 ReaderRelay 有每请求 Case 隔离；基础类无其他可达并发调用方 | 不修改；离线并发替身未构造出真实调用方错归属 |

## 剩余限制

- 定时工作流（cron）尚未在默认分支 main 安装，需维护者完成 main 集成授权后生效；手动触发不等于 schedule 事件已发生。
- 最终 Tag、`submission.yaml`、提交邮件需维护者授权阶段执行；本阶段不产生发布 Tag。
- PostgreSQL 专用契约测试因环境无 Docker 未执行；CI 大体积历史产物未逐字节核验。
- 云解析费用未知保持 null；共享预算 USD 20 累计账本本轮零新增调用。
