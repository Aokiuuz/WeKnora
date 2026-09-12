# 源要求与成果对应

功能与验收依据为《WeKnora 开源实战课题.docx》课题三；提交格式依据为《WeKnora_腾讯犀牛鸟_课题实战流程指引_2026.docx》。下列编号用于连接要求、实现与可观察结果。

| 编号 | 源要求 | 实现与成果 |
|---|---|---|
| T1 | 固定数据集、模型及分块参数，结果入库 | 数据版本、实验快照、租户任务及逐题结果持久化；持久取消与租约回收 |
| T2 | 每轮检索准确性、答案质量、成本和耗时 | 检索与生成指标计划、模型调用账本、运行详情及结构化导出 |
| T3 | 定时持续集成及退化阻断合并 | 默认分支定时回归与性能工作流；受保护分支必需固定指标检查；生产召回退化退出1 |
| T4 | 每次模型调用记录及模型页展示 | modelobs调用包装、数据库聚合、按模型与时间的用量面板 |
| T5 | 相同文本与模型复用嵌入 | 租户、模型、配置和文本摘要构成缓存身份；冷轮25次调用、热轮0次 |
| T6 | 共享提示词固定前缀及缓存对照 | Wiki资料与固定指令前置；三批各30对的缓存令牌、费用和耗时统计 |
| A1 | 干净环境单命令复现 | make evaluation-verify；13项固定指标及完整服务、缓存与任务恢复检查 |
| A2 | 召回下降使持续集成报错 | 生产融合路径保留最差候选，recall=0触发下限失败并退出1 |
| A3 | 模型页按模型和时间查询 | 数据库存储与界面筛选对应；前端921项测试、34布局场景及5项按钮操作 |
| A4 | 两类缓存前后对比 | 嵌入25→0；Wiki固定前缀组费用比0.533/0.619/0.631及逐对缓存数据 |
| O1 | 同批文件的八解析引擎质量基线 | 100PDF×8；636成功、160错误、4空输出；正文覆盖、文本、表格、顺序及公式字符串指标 |

核心模块位于准确源码的internal/application/service/evaluation*、internal/modelobs、internal/modelcache、internal/application/repository/model_statistics.go、frontend/src/views/evaluation/EvaluationWorkbench.vue和frontend/src/components/ModelUsageDrawer.vue。工作流为.github/workflows/evaluation-regression.yml与evaluation-performance.yml；解析入口为cmd/parser-benchmark及scripts/run-parser-benchmark.py。

复现命令见[REPRODUCE.md](REPRODUCE.md)，统计条件与版本映射见[EXPERIMENTS.md](EXPERIMENTS.md)和[versions.json](versions.json)。任务恢复发布中断或取消终态；定时配置、手动运行和拉取请求检查按各自事件记录，首次定时事件在材料观察时刻尚未出现。

源文件SHA-256：课题要求3a2bc7a3a58275a91b456e96883805c5293c921f79903f0dda888f46351ef77b；流程指引bc84cde405de2c79caccfd9c271d6936004f8cf04df49fdcf94dc9025040b099。
