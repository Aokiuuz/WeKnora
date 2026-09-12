# 质量评测基线与成本可观测

腾讯犀牛鸟开源实战课题三 · 刘蔚霖 · 南开大学 · GitHub Aokiuuz

系统在WeKnora中实现持久化评测工作台、固定质量指标与回归门禁、模型调用账本、按模型和时间的用量统计、持久化嵌入缓存及Wiki固定前缀。八解析引擎采用同批100份PDF形成800条结果的横向基线。

| 阅读入口 | 内容 |
|---|---|
| [技术报告](TECHNICAL-REPORT.pdf) | 架构、实现、实验图表与结果 |
| [源要求与成果对应](ACCEPTANCE.md) | 六项必做、四项验收与八引擎选做 |
| [复现手册](REPRODUCE.md) | 环境、单命令及界面功能查看 |
| [实验方法与数据](EXPERIMENTS.md) | 样本分母、计算口径和执行版本 |
| [版本清单](versions.json) | 源码及各实验的准确提交 |
| [文件摘要](checksums.sha256) | 附件文件完整性 |

代码仓库：[Aokiuuz/WeKnora](https://github.com/Aokiuuz/WeKnora)。代码分支：main。完整提交：[`ad3330d549adc2c17d0193d22dcdcecf69a7ab12`](https://github.com/Aokiuuz/WeKnora/tree/ad3330d549adc2c17d0193d22dcdcecf69a7ab12)。

单命令复现：在准确Git检出与所列依赖就绪后运行 `make evaluation-verify`。

已提交官方贡献：[增量lint修复](https://github.com/Tencent/WeKnora/pull/3221)、[检索来源映射](https://github.com/Tencent/WeKnora/pull/2851)、[完整功能集成参考](https://github.com/Tencent/WeKnora/pull/3220)。前两项等待维护者审阅，完整集成为草稿合并请求。
