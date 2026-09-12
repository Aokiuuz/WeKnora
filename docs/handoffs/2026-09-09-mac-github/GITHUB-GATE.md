# GitHub 质量门禁验收

目标仓库为 `Aokiuuz/WeKnora`，目标分支为 `main`。拉取请求（Pull Request，PR）中的代码运行持续集成（Continuous Integration，CI），分支保护将该结果作为合并条件。本文件给出后续验收步骤；实际远端写入按对应授权执行。

## 正常 PR

1. 在 Mac 核对 gh 登录账号、仓库推送和管理员权限；读取远端分支、默认分支、Actions 及规则。分支上传不会自动完成门禁验收。
2. 完整审查功能分支到 `main` 的差异，记录 PR 源提交和目标提交。该交付包含上游整合和课题三功能，应按实际全部差异审查。
3. 获得创建 PR 的授权后，创建普通 PR，从 `feat/public-dataset-workbench` 合并到同仓库 `main`。合并操作另行确认。
4. 等待 `.github/workflows/evaluation-regression.yml` 的 `Evaluation regression` 工作流，其任务名为 `Deterministic golden evaluation`。该工作流包含黄金样本、公开数据和持久化 HTTP 回归，产物名为 `evaluation-regression-运行编号`。
5. 保存成功运行和完整产物。分别记录 PR 源提交与实际测试提交；PR 工作流可以测试 GitHub 生成的合并提交。

质量工作流触发器为 `pull_request`、`schedule`、`workflow_dispatch`。仅推送普通功能分支不会触发该质量工作流。PR 存在合并冲突、提交信息跳过 CI、Actions 需要首次批准等情况时，先处理对应原因。[GitHub 工作流触发排查](https://docs.github.com/en/actions/how-tos/troubleshoot-workflows)

## 必需检查

取得本仓库一次近期成功的质量运行后，展示并授权设置以下 `main` 分支规则：要求 PR、要求状态检查、选择实际观察到的 `Deterministic golden evaluation` 并绑定 GitHub Actions 来源、要求与目标分支保持最新、规则对管理员生效。保留现有有效规则，按实际参与者设置审批数量。

GitHub 候选必需检查应在过去七天内成功运行过。使用仓库 Settings → Branches 中的保护规则，读取保存后的接口响应并留存截图。规则项、目标分支和生效状态必须可核对。[必需检查要求](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks)，[保护规则设置](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/managing-a-branch-protection-rule)

## 受控下降与恢复

在独立干净克隆中，从正常交付提交创建测试分支。对 `internal/retrieval/fusion/fusion.go` 的 `Results` 函数进行单处业务改动，在 `result := withRRF(...)` 后加入：

```go
// Controlled regression demonstration: remove the highest ranked result.
if len(result) > 0 {
    result = result[1:]
}
```

保持数据、阈值、断言及工作流不变，确认差异仅包含该故障注入。该改动使倒数排名融合（Reciprocal Rank Fusion，RRF）的首项丢失，固定黄金样本参考召回率从 0.75 降到 0.25。本次指标仍以实际产物为准；直接覆盖指标数值不能证明检索行为受门禁保护。

经授权推送测试分支并建立指向受保护 `main` 的普通 PR。验收同时要求质量工作流失败、日志明确报告退化指标、PR 无冲突且非草稿、合并区明确因该必需检查而禁止合并、当前账号受保护规则约束。保存失败产物、提交、运行链接及阻断截图，不通过点击合并按钮试探。

对故障提交追加 `git revert` 恢复提交并推送，保存质量检查重新通过、指标恢复及该检查阻断消失的证据。失败提交和报告完整保留。测试 PR 的合并不属于故障验收要求。

## 定时运行

定时表达式 `23 3 * * 1` 对应每周一协调世界时（Coordinated Universal Time，UTC）03:23、北京时间 11:23。`schedule` 在默认分支上运行，`workflow_dispatch` 也要求工作流文件存在于默认分支。功能分支上传与 PR 成功均不能替代默认分支定时部署。正常合并单独确认，之后核对默认分支文件和工作流启用状态，并保留一次真实 `schedule` 事件。[GitHub 事件规则](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)

表 1 列出平台证据应具备的关联信息。

| 阶段 | 证据 |
| --- | --- |
| 正常运行 | PR、源与目标完整提交、实际测试提交、运行编号、工作流文件身份、指标与数据摘要 |
| 分支保护 | 精确目标、检查名称及来源、管理员约束、实际规则响应 |
| 故障运行 | 单一故障补丁、失败提交、失败指标、运行链接、质量检查阻断截图 |
| 恢复运行 | revert 提交、成功指标、运行链接、质量阻断消失截图 |
| 定时运行 | 默认分支提交、真实 schedule 事件、实际触发时间及运行结果 |

表中的源提交、运行和报告需对应。截图颜色仅为辅助证据，原始报告和有效规则共同证明门禁行为。
