# 运行与复现

运行环境为Linux或Windows Subsystem for Linux（WSL，适用于Linux的Windows子系统）的原生文件系统。依赖为Git、Go 1.26、GCC/G++、GNU Make和Python 3.11+。前端使用Node.js/npm，已验证环境为Node 24。

## 单命令工程验收

```sh
git clone https://github.com/Aokiuuz/WeKnora.git WeKnora-topic3
cd WeKnora-topic3
git checkout --detach ad3330d549adc2c17d0193d22dcdcecf69a7ab12
make evaluation-verify
```

该命令自动创建合成供应商与隔离SQLite数据，模型调用使用回环服务。首次依赖下载需要网络，工程验收无需模型账号。运行前保持工作树干净，并空出18708、18710、18908、18910回环端口。

输出位于仓库旁evaluation-evidence/acceptance.*/verification-summary.json。预期退出0，status为passed，code_commit对应所检出提交，golden_checks包含13项固定指标；嵌入调用冷轮25、热轮0；故障终态为6（取消）和5（中断）；paid_provider_requests为0。完整后端实测来源及当前前端对应关系见[版本清单](versions.json)。

## 前端构建及功能查看

```sh
cd frontend
npm ci
npm test
npm run type-check
npm run build
```

已验证结果为921项测试通过，类型检查与构建退出0。应用启动及模型、知识库配置参见准确代码中的README.md与docs/EVALUATION_WORKBENCH.md。依赖目录在当前操作系统中按锁文件安装。

进入评测工作台后，可以创建固定配置实验、查看逐题答案与来源、导出JSON（JavaScript Object Notation）或CSV（Comma-Separated Values）文件，并比较兼容的两轮结果。模型设置的用量面板支持模型与时间范围筛选。图示见技术报告。

## 解析与规模基准

```sh
python3 -B scripts/test_parser_benchmark.py ScoringIntegrityTests OfficialDenominatorAuditTests ReviewBindingTests ManifestMetadataTests
python3 -B scripts/test_parser_benchmark_launcher.py
python3 -B scripts/test_parser_benchmark_cpu_batch.py
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -tags sqlite_fts5 ./internal/application/repository -run '^$' -bench BenchmarkModelStatisticsScale -benchtime=1x -count=1
```

三条解析离线命令分别预期15、4、11项通过。完整解析输入按dataset/parser-benchmark/README.md取得100份PDF及参考文件；重新调用云解析接口使用显式凭据和对应预算。数据库升级与PostgreSQL契约说明位于docs/EVALUATION_WORKBENCH.md。

费用、真实模型样本和机器评分采用[实验方法](EXPERIMENTS.md)中的口径。源码以完整Git提交获取，数据附件用于核对报告结果。
