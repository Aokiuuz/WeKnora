# 验收演示与交付索引

## 本机演示

1. 双击项目根目录 `Start-WeKnora.cmd`，等待五个容器健康，打开 `http://127.0.0.1:5174`。
2. 使用本机已有账号进入模型管理，查看四个对话模型、两个向量模型及美元价格版本。
3. 打开知识库“验收示例（合成资料）”，查看已解析的 `acceptance-example.md`。材料包含座位数、开放时间和预约条件。
4. 在评测工作台查看“个人服务验收：2题合成资料”的完成任务，检查逐题回答、检索结果、耗时、用量与费用，再下载 JSON 或 CSV 导出文件。JSON（JavaScript Object Notation）保存结构化结果，CSV（Comma-Separated Values，逗号分隔值）用于表格分析。
5. 查看模型统计与调用账本。成功调用保留供应商用量、价格快照和实际费用；未确定费用保留对应状态。
6. 打开[扩展实测报告](reports/expanded-acceptance/README.md)，展示三套公开数据的质量检查、两个模型的回答质量、完整检索流程、跨批次页面生成缓存及浏览器证据。[持久化与故障实测](reports/final-acceptance/README.md)提供五次重启缓存对照、内容修改探测和取消恢复证据。
7. 展示[正常交付变更](https://github.com/Aokiuuz/WeKnora/pull/1)和[受控退化变更](https://github.com/Aokiuuz/WeKnora/pull/2)。后者保留检查失败及合并阻断证据，处于关闭状态。

现有评测记录可以直接查看。重新执行评测或发起问答会按所选供应商计费。

## 模块与证据

| 能力 | 实现入口 | 验证证据 |
| --- | --- | --- |
| 固定数据评测与逐题结果 | `internal/application/service/evaluation*`、评测工作台 | 三个公开开发集质量检查、两模型 1,200 份原始上下文回答；完整检索流程独立统计 |
| 供应商费用记账 | `internal/modelobs`、`internal/types/provider_cost.go` | 实际费用、账本及导出逐条对账，金额边界测试 |
| 持久化向量缓存 | `internal/modelcache` | 五组冷启动与进程重启对照、内容修改探测、500 段中途失败与续跑回归 |
| Wiki 提示词缓存 | 页面生成流程、`cmd/evaluation-provider-probe` | 固定路由、独立前缀批次的费用与耗时配对数据 |
| 取消与异常恢复 | 评测任务状态、执行租约 | HTTP 取消、重复取消、强制结束进程及恢复 |
| 预算约束 | `scripts/evaluation-openrouter-acceptance.py` | 预留、结算、跨进程锁、未知费用和收据核对，十项零网络测试 |
| 回归门禁 | `.github/workflows/evaluation-regression.yml` | 正常检查、受控退化、必需检查配置 |
| 个人完整服务 | `scripts/personal-service.ps1` | 五个健康容器、模型价格、文档解析及两题评测 |
| 浏览器交互 | `scripts/evaluation-browser-acceptance.cjs` | 12 项真实页面检查，包含筛选、详情、文件下载与设置弹层 |

上述实现与证据按模块组织。源代码提交记录保存变更作者与提交身份；Apple M4 工程复现保留独立证据包及其提交范围。

## 交付边界

公开报告包含脱敏汇总、回答、机器评分、配对结果与文件摘要。本机证据目录保存原始数据库、导出、收据、失败批次和预算文件，涉及账号及运行环境的数据按本地资料保管。

本轮人工评分暂不执行，字段保持空值。公开参考答案用于自动比对，模型辅助意见保留机器评分身份。无答案拒答准确性、公开数据的预训练重叠风险、完整仓库静态检查结果，以及 Apple M4 证据的具体源码范围见扩展实测报告。
