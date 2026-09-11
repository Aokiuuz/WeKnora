"""Build inspectable static research figures and acceptance facts from raw runs.

Dependencies: numpy, matplotlib. No network, paid calls or human scoring.
"""
import argparse
from collections import Counter
from decimal import Decimal, ROUND_HALF_UP
import hashlib
import json
from pathlib import Path
import shutil

import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

MODELS = ['deepseek/deepseek-v4-flash', 'moonshotai/kimi-k2.5']
NAMES = ['DeepSeek V4 Flash', 'Kimi K2.5']
DATASETS = ['cmrc', 'squad', 'hotpot']
DATA_NAMES = ['CMRC2018', 'SQuAD 2.0', 'HotpotQA']
COLORS = ['#176889', '#c78324']


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def cluster_interval(rows, field, iterations=5000):
    groups = sorted({r['group'] for r in rows})
    sums = np.array([sum(r['score'][field] for r in rows if r['group'] == g) for g in groups])
    counts = np.array([sum(r['group'] == g for r in rows) for g in groups])
    rng = np.random.default_rng(20260911)
    samples = rng.integers(0, len(groups), size=(iterations, len(groups)))
    boot = sums[samples].sum(axis=1) / counts[samples].sum(axis=1)
    return list(map(float, np.percentile(boot, [2.5, 97.5])))


def reader(root, out):
    status = read(root / 'reader-holdout/status.json')
    assert status['successful'] == status['outputs'] == 1200
    plan = read(root / 'reader-holdout/plan.json')
    assert plan['cases_sha256'] == sha(root / 'data/holdout.json')
    cases = read(root / 'data/holdout.json')
    truth = {(q['dataset'], q['qid']): q for q in cases}
    rows = read(root / 'reader-holdout/results.json')
    assert len({(r['dataset'], r['qid'], r['model']) for r in rows}) == 1200
    scores = []
    for row in rows:
        assert row['status'] == 'success'
        ledger, receipt = row['ledger'][0], row['receipt']
        exact = Decimal(str(receipt['usage']['cost']))
        assert ledger['accounting_complete'] and ledger['status'] == 'success'
        assert ledger['cost_microunits'] == int((exact * 1000000).quantize(Decimal(1), rounding=ROUND_HALF_UP))
        assert row['build']['commit_id'] == plan['source_commit']
        scores.append({k: row[k] for k in ['dataset', 'qid', 'type', 'group', 'model', 'score', 'raw_answer']} |
                      {'references': truth[row['dataset'], row['qid']]['answers'], 'cost_usd': str(exact),
                       'latency_ms': ledger['duration_ms'], 'attempts': len(row['attempts']), 'human_rating': None})
    write(out / 'reader-scores.json', scores)
    summaries = []
    for dataset in DATASETS:
        for model in MODELS:
            subset = [r for r in rows if r['dataset'] == dataset and r['model'] == model]
            no_answer = [r for r in subset if r['type'] == 'unanswerable']
            answerable = [r for r in subset if r['type'] != 'unanswerable']
            summaries.append({'dataset': dataset, 'model': model, 'n': len(subset),
                'groups': len({r['group'] for r in subset}), 'em': float(np.mean([r['score']['em'] for r in subset])),
                'f1': float(np.mean([r['score']['f1'] for r in subset])),
                'em_ci95': cluster_interval(subset, 'em'), 'f1_ci95': cluster_interval(subset, 'f1'),
                'unanswerable_n': len(no_answer), 'correct_refusals': sum(r['score']['em'] for r in no_answer),
                'false_refusals': sum(r['score']['refused'] for r in answerable), 'answerable_n': len(answerable),
                'invalid_json': sum(not r['score']['format_valid'] for r in subset),
                'attempts': sum(len(r['attempts']) for r in subset),
                'successful_call_usd': str(sum(Decimal(str(r['receipt']['usage']['cost'])) for r in subset)),
                'p50_ms': float(np.percentile([r['ledger'][0]['duration_ms'] for r in subset], 50)),
                'p95_ms': float(np.percentile([r['ledger'][0]['duration_ms'] for r in subset], 95))})
    fig, axes = plt.subplots(1, 2, figsize=(11.8, 4.4), layout='constrained')
    for ax, metric, title in zip(axes, ['em', 'f1'], ['完全匹配 EM', '词元重合 F1']):
        for mi, model in enumerate(MODELS):
            vals = [next(s for s in summaries if s['dataset'] == d and s['model'] == model) for d in DATASETS]
            y = np.array([s[metric] for s in vals])
            bounds = np.array([s[metric + '_ci95'] for s in vals])
            x = np.arange(3) + (mi - .5) * .24
            ax.errorbar(x, y, yerr=[y - bounds[:, 0], bounds[:, 1] - y], fmt=['o', 's'][mi],
                        color=COLORS[mi], label=NAMES[mi], capsize=5, markersize=7, linewidth=1.5)
            for index, (xi, yi) in enumerate(zip(x, y)):
                bound = bounds[index, 1 if mi == 0 else 0]
                ax.annotate(f'{yi:.1%}', (xi, bound), xytext=(0, 9 if mi == 0 else -16), textcoords='offset points', ha='center', fontsize=9)
        ax.set_xticks(range(3), DATA_NAMES); ax.set_xlim(-.5, 2.5); ax.set_ylim(0, 1.04)
        ax.set_yticks([0, .25, .5, .75, 1], ['0%', '25%', '50%', '75%', '100%'])
        ax.set_title(title); ax.grid(axis='y', alpha=.2); ax.set_axisbelow(True)
    axes[0].legend(loc='lower left', frameon=False)
    fig.suptitle('原始上下文回答 · 每个数据集 200 题 × 2 模型', fontsize=14)
    fig.supxlabel('误差线：按来源分组进行 5,000 次自助重采样，取 95% 百分位区间', fontsize=9)
    fig.savefig(out / 'reader-quality.png', dpi=180); plt.close(fig)
    fig, ax = plt.subplots(figsize=(8.8, 4.3), layout='constrained')
    subset = [s for s in summaries if s['dataset'] == 'squad']
    for i, s in enumerate(subset):
        correct = s['correct_refusals']
        ax.barh(i, correct, color=COLORS[i], height=.55)
        ax.barh(i, 100 - correct, left=correct, color='#e6eaed', hatch='///', edgecolor='#bec7cd', height=.55)
        ax.text(correct / 2, i, f'{int(correct)} / 100', va='center', ha='center', color='white', fontweight='bold')
        ax.text(correct + (100 - correct) / 2, i, f'未正确拒答 {int(100-correct)}', ha='center', va='center', fontsize=10)
    ax.set_yticks([0, 1], NAMES); ax.invert_yaxis(); ax.set_xlim(0, 100)
    ax.set_xlabel('问题数'); ax.set_title('SQuAD 2.0 · 原始段落无法回答的 100 题', pad=14)
    fig.savefig(out / 'unanswerable.png', dpi=180); plt.close(fig)
    write(out / 'reader-summary.json', {'source_commit': plan['source_commit'], 'binary_sha256': plan['binary_sha256'],
          'summaries': summaries, 'uncertainty': '5000 source-group bootstrap resamples; seed 20260911; 95% percentile intervals',
          'human_review': 'excluded'})
    return summaries, plan


def wiki_batches(root, baseline, out):
    pairs, summaries = [], []
    for batch, directory in [('A', baseline / 'probes-01'), ('B', root / 'wiki-b'), ('C', root / 'wiki-c')]:
        assert read(directory / 'status.json')['status'] == 'passed'
        data = read(directory / 'results.json')
        current = []
        for number in range(1, 31):
            pair = {'batch': batch, 'pair': number}
            for arm in ['stable-prefix', 'page-first']:
                result = next(r for r in data if r['step'] == f'wiki-{number:02d}-{arm}')
                ledger = result['ledger'][0]
                assert ledger['accounting_complete'] and ledger['status'] == 'success'
                cost = Decimal(ledger['model_snapshot']['billing_usage']['reported_cost'])
                assert ledger['cost_microunits'] == int((cost * 1000000).quantize(Decimal(1), rounding=ROUND_HALF_UP))
                pair[arm] = {'cache_read_tokens': ledger['provider_cache_read_tokens'],
                    'prompt_tokens': ledger['prompt_tokens'], 'cost_usd': float(cost),
                    'duration_ms': ledger['duration_ms'], 'checks': result['fact_checks']}
            current.append(pair)
        summary = {'batch': batch, 'pairs': len(current), 'plan_sha256': sha(directory / 'plan.json')}
        for field in ['cache_read_tokens', 'cost_usd', 'duration_ms']:
            summary[field + '_stable_minus_page_mean'] = float(np.mean([p['stable-prefix'][field] - p['page-first'][field] for p in current]))
        for arm in ['stable-prefix', 'page-first']:
            summary[arm] = {'cost_usd': sum(p[arm]['cost_usd'] for p in current),
                'duration_p50_ms': float(np.median([p[arm]['duration_ms'] for p in current])),
                'duration_p95_ms': float(np.percentile([p[arm]['duration_ms'] for p in current], 95)),
                'checks_passed': sum(all(p[arm]['checks'].values()) for p in current)}
        summary['cost_ratio'] = summary['stable-prefix']['cost_usd'] / summary['page-first']['cost_usd']
        summary['latency_p50_ratio'] = summary['stable-prefix']['duration_p50_ms'] / summary['page-first']['duration_p50_ms']
        pairs.extend(current); summaries.append(summary)
    write(out / 'wiki-pairs.json', pairs)
    fig, axes = plt.subplots(1, 3, figsize=(12, 4.1), layout='constrained')
    for ax, field, title, reference in zip(axes,
            ['cache_read_tokens_stable_minus_page_mean', 'cost_ratio', 'latency_p50_ratio'],
            ['平均缓存读取令牌差', '总费用比：稳定前缀 / 页面优先', '中位耗时比：稳定前缀 / 页面优先'], [0, 1, 1]):
        values = [s[field] for s in summaries]
        ax.bar(range(3), values, width=.5, color='#176889')
        ax.axhline(reference, color='#404f59', linewidth=1, linestyle='--')
        ax.set_xticks(range(3), ['批次 A', '批次 B', '批次 C'])
        ax.set_title(title, fontsize=10); ax.grid(axis='y', alpha=.18); ax.set_axisbelow(True)
        ax.set_ylim(min(0, min(values) * 1.2), max(max(values), reference) * 1.25)
        for i, value in enumerate(values):
            ax.annotate(f'{value:.1f}' if field.startswith('cache') else f'{value:.2f}', (i, value),
                        xytext=(0, 7), textcoords='offset points', ha='center')
    fig.suptitle('Wiki 页面生成 · 3 批次 × 30 对 · 固定供应商路由', fontsize=14)
    fig.savefig(out / 'wiki-batches.png', dpi=180); plt.close(fig)
    return summaries


def render_report(facts):
    reader_rows = '\n'.join(
        f"| {DATA_NAMES[DATASETS.index(s['dataset'])]} | {NAMES[MODELS.index(s['model'])]} | {s['n']} | "
        f"{s['em']:.1%} | {s['f1']:.1%} | {s['f1_ci95'][0]:.1%}–{s['f1_ci95'][1]:.1%} | {s['invalid_json']} |"
        for s in facts['reader'])
    http_rows = '\n'.join(
        f"| {r['dataset'].upper()} | {NAMES[MODELS.index(r['model'])]} | {r['questions']} / {r['passages']} | "
        f"{r['recall']:.1%} | {r['ndcg3']:.4f} | {r['cost_usd']:.6f} | {r['accounting']['unknown_cost_attempts']} | {r['empty_outputs']} |"
        for r in facts['http'])
    wiki_rows = '\n'.join(
        f"| {s['batch']} | {s['pairs']} | {s['cache_read_tokens_stable_minus_page_mean']:.1f} | {s['cost_ratio']:.3f} | "
        f"{s['latency_p50_ratio']:.3f} | {s['stable-prefix']['checks_passed']}/30 / {s['page-first']['checks_passed']}/30 |"
        for s in facts['wiki'])
    budget = facts['budget']
    return f'''# 扩展验收实测报告

系统提供固定数据评测、逐题证据、供应商用量账本、持久化向量缓存、页面生成和一键启动。验收证据包括 1,200 份原始上下文回答、200 条完整检索流程结果、90 组页面生成配对数据及 12 项真实浏览器检查。人工评分字段为空。

## 系统与验证范围

检索增强生成（Retrieval-Augmented Generation，RAG）流程从固定数据版本读取段落和问题，建立关键词与向量索引，融合检索结果，再调用所选模型生成答案。每次供应商物理请求写入调用账本，评测任务聚合检索、延迟、用量和费用数据。结构化文本格式 JSON（JavaScript Object Notation）与逗号分隔值格式 CSV（Comma-Separated Values）导出保存一致的运行指标。

下图展示两类质量实验的输入路径和共享观测模块。

```mermaid
flowchart LR
    A[固定公开数据与来源摘要] --> B[原始上下文＋问题]
    A --> C[500 段语料＋50 个问题]
    C --> D[关键词与向量检索]
    D --> E[生产聊天适配器与证据规则]
    B --> E
    E --> F[逐题回答与自动比对]
    E --> G[物理请求账本与预算预留]
    F --> H[图表、逐题结果、导出]
    G --> H
```

原始上下文实验衡量模型从已提供材料回答问题的能力。完整流程实验经过超文本传输协议（Hypertext Transfer Protocol，HTTP）接口，包含索引、检索、生成和任务落库。两类结果分别统计。

## 数据来源与质量

三个公开开发集共读取 22,497 题。结构、答案位置、支持句索引与重复标识检查保留 22,320 题，排除 177 题。来源为 [CMRC2018 官方仓库](https://github.com/ymcui/cmrc2018)、[SQuAD 官方项目](https://rajpurkar.github.io/SQuAD-explorer/)和 [HotpotQA 官方项目](https://hotpotqa.github.io/)；HotpotQA 文件使用作者组织的[固定版本镜像](https://huggingface.co/datasets/hotpotqa/hotpot_qa/tree/1908d6afbbead072334abe2965f91bd2709910ab)。版本、许可文件、下载地址和 SHA-256 摘要见 [sources.json](sources.json)。SHA-256 是安全散列算法 256 位版本，用于验证文件内容一致性。

| 数据集 | 读取题数 | 有效题数 | 排除题数 | 调试样本 | 正式样本 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 中文机器阅读理解 CMRC2018 | 3,219 | 3,043 | 176 | 32 | 200 |
| 斯坦福问答数据集 SQuAD 2.0 | 11,873 | 11,873 | 0 | 32 | 200 |
| 多跳问答数据集 HotpotQA | 7,405 | 7,404 | 1 | 32 | 200 |

固定种子为 `20260911-excellence-v1`。调试与正式样本的来源分组和上下文内容互不重叠，并排除前次 24 题实验已使用的上下文。正式样本包含 SQuAD 的 100 道可回答题与 100 道无答案题，以及 HotpotQA 的 100 道桥接题与 100 道比较题。公开开发集可能出现在模型预训练材料中；这些结果描述固定过滤子集，不能替代官方榜单成绩。

## 原始上下文回答

96 道调试题形成 192 份提示词对照输出，规则选择以预先保存的宏平均 F1 和拒答条件为依据。正式集固定后，两个模型各回答 600 题，共完成 1,200 份输出和 1,211 次尝试。失败尝试保留于原始证据；6 份格式无效的回答按零分计入正式样本。

完全匹配（Exact Match，EM）衡量标准化答案是否与任一参考答案一致。F1 为词元重合的精确率与召回率的调和平均。英文采用 SQuAD 标准化规则，中文采用汉字与拉丁词分段；HotpotQA 的是非答案不匹配时计零。中文分词方案属于本实验口径。

| 数据集 | 模型 | 题数 | EM | F1 | F1 的 95% 区间 | 格式错误 |
| --- | --- | ---: | ---: | ---: | --- | ---: |
{reader_rows}

下图给出点估计与来源分组重采样区间。

![原始上下文回答质量](reader-quality.png)

误差线使用 5,000 次来源分组自助重采样，固定种子为 20260911，取第 2.5 与第 97.5 百分位。SQuAD 正式集来自 12 个来源分组，同组题目存在关联。图中的区间描述该采样方案下的不确定性。原始回答、全部参考答案、自动得分和逐条成功调用费用见 [reader-scores.json](reader-scores.json)。

下图展示 SQuAD 原始段落无答案题的拒答结果。

![无答案问题拒答](unanswerable.png)

DeepSeek V4 Flash 正确拒答 43/100，Kimi K2.5 正确拒答 29/100。证据不足时给出答案是当前可测量的质量弱项。这里的无答案标签适用于原始段落；合并语料后的可回答性需要独立判断。

## 完整检索流程

每个数据集使用 500 个真实段落与 50 道可回答问题，每个模型完成一轮。相关性标签标记问题的原始证据段落，语料中可能存在其他语义相关段落。召回率按这些固定标签计算；归一化折损累计增益前三位（Normalized Discounted Cumulative Gain at 3，NDCG@3）衡量前三项的排序质量。

| 数据集 | 模型 | 问题 / 段落 | 召回率 | NDCG@3 | 已知费用 USD | 费用未知尝试 | 空输出 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
{http_rows}

供应商嵌入批次包含 5 段，嵌入模型与执行池并发均为 4。CMRC 的 DeepSeek 任务使用 1 个问题工作线程，其余任务使用 4 个；运行资源差异限制了任务总耗时的直接比较。每个评测任务均核对数据库、详情接口、JSON 与 CSV 的运行指标。成功调用费用按供应商收据逐条转换为百万分之一美元并核对总和；网络失败或供应商未报告用量时，未知金额保持空值。空输出按零分计入自动质量指标，表中的空输出列保留其数量。

持久化向量缓存按租户、模型、行为参数和文本内容区分条目。池化嵌入每 64 段保存一轮进度，后续窗口失败时已保存窗口可供重试和进程重启使用。500 段回归覆盖中途失败、重启复用、重复文本顺序、返回向量独立性及取消停止后续调用。五组真实冷启动与重启对照、内容修改探测见[缓存实测证据](../final-acceptance/README.md)。

## 页面生成缓存、费用与耗时

Wiki 知识页面生成使用 DeepSeek V4 Flash，固定供应商路由 `gmicloud/fp8`。每批 30 对请求交替安排稳定前缀与页面优先两种布局；批次 A、B、C 使用独立的前缀命名空间。四项自动事实检查覆盖座位数、目录项、开放时间和未经支持的每日开放描述。

| 批次 | 配对数 | 缓存读取令牌平均差 | 总费用比 | 中位耗时比 | 四项全通过：稳定 / 页面 |
| --- | ---: | ---: | ---: | ---: | --- |
{wiki_rows}

缓存令牌差为稳定前缀减页面优先；两项比值均为稳定前缀除以页面优先。下图按批次展示这三个指标。

![页面生成配对结果](wiki-batches.png)

缓存收益随批次变化；同批请求共享供应商缓存状态，配对样本不能视为完全独立。费用与耗时采用实际记录，四项自动检查属于有限规则覆盖。逐对数据见 [wiki-pairs.json](wiki-pairs.json)。

## 工程与页面验收

真实浏览器完成 12 项检查：四类指标、JSON 下载、CSV 下载、逐题证据、状态与数据集筛选、空结果、筛选复位、设置页单一弹层、延迟与缓存缺失值、模型筛选、自定义时间范围和页面异常。下图展示逐题回答与检索证据。

![逐题回答与检索证据](02-question-evidence.png)

页面截图来自本机两个合成问题的固定演示任务，用于验证界面行为。大规模质量结论来自上述独立数据实验。

文件写入同时检查复制与关闭错误；配置读取拒绝无效展开内容；批量知识访问传播数据库错误并处理空代理；审批订阅退避在接收实际消息后复位。对应回归测试和本轮增量静态检查通过。项目全量根模块静态扫描记录 5,923 项发现，其内容含代码格式、长行、错误处理与静态分析建议；全量扫描状态与本轮增量状态分别保存，不能据增量通过宣称全仓检查通过。

固定评测门禁、持久化 HTTP 冷热缓存、取消、重复取消、进程终止与执行租约恢复均保留零费用回归证据。[交付变更](https://github.com/Aokiuuz/WeKnora/pull/1)保存持续集成结果；[受控退化变更](https://github.com/Aokiuuz/WeKnora/pull/2)保存召回退化触发必需检查失败及阻断合并的证据。

## 版本、预算与复现

原始上下文探针源码提交为 `{facts['reader_plan']['source_commit']}`，探针摘要为 `{facts['reader_plan']['binary_sha256']}`。当前个人服务报告的源码提交为 `{facts['personal_build']['commit_id']}`。完整检索任务的构建标识见 [summary.json](summary.json)，各实验保留实际执行二进制的来源，不以报告提交代替实验提交。

Apple M4 工程复现使用提交 `1776aa442a491f1657196ca21da914bab197b17c`。提供的独立证据包汇总记录两次新建数据库运行和 988 项一致性检查通过，证据审阅范围见本机交付索引。当前扩展实验在 Linux amd64 环境执行，M4 记录的适用范围以其提交标识为准。

模型累计预算为 20 美元，当前供应商已报告费用 `{budget['reported_usd']}` 美元，未结算调用保守预留 `{budget['unsettled_reserved_usd']}` 美元，共 `{budget['requests']}` 次预算记录。预算覆盖本轮、前次实验和失败重试。每次出站调用先预留成本，已知费用按实际结算，未知费用继续占用预算；文件锁阻止两个付费执行器同时使用该预算文件。

数据准备、质量评分、HTTP 执行与报告生成脚本位于 `scripts/prepare-expanded-evaluation.py`、`scripts/prepare-expanded-retrieval.py`、`scripts/evaluation-expanded-reader.py`、`scripts/evaluation-expanded-http.py` 和 `scripts/build-excellence-report.py`。付费脚本在隔离 Linux 容器中从标准输入接收凭据，共用同一预算文件。数据库运行于容器内部，应用停止后导出一致性备份。失败批次 `http-01`、`http-02` 保留中止原因和费用，不参与有效质量汇总。

本机一键启动入口、模型查看、现有演示任务与导出步骤见[验收演示索引](../../acceptance-demo.md)。人工评分与人工主观质量结论均未生成。
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--evidence', type=Path, required=True)
    parser.add_argument('--baseline', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--reader-only', action='store_true')
    parser.add_argument('--http-directory', default='http-04')
    args = parser.parse_args(); root, out = args.evidence, args.output
    out.mkdir(parents=True, exist_ok=True)
    plt.rcParams.update({'font.family': 'Microsoft YaHei', 'font.size': 10,
        'axes.spines.top': False, 'axes.spines.right': False, 'axes.edgecolor': '#b5bfc6',
        'text.color': '#253744', 'axes.labelcolor': '#253744', 'figure.facecolor': 'white'})
    summaries, plan = reader(root, out)
    if args.reader_only:
        return
    wiki = wiki_batches(root, args.baseline, out)
    http_root = root / args.http_directory
    manifest = read(http_root / 'manifest.json'); assert manifest['status'] == 'passed'
    runs = []
    import csv
    for run in manifest['rounds']:
        run_root = root / run.get('evidence_directory', args.http_directory)
        data = read(run_root / (run['label'] + '.json'))
        with (run_root / (run['label'] + '.csv')).open(encoding='utf-8-sig', newline='') as stream:
            record = next(row for row in csv.DictReader(stream) if row['record_type'] == 'run')
        assert json.loads(record['runtime_metrics_json']) == data['runtime_metrics']
        runs.append({'dataset': run['label'].split('-')[0], 'model': run['model'], 'questions': run['questions'],
                     'passages': 500, 'recall': run['metric']['retrieval_metrics']['recall'],
                     'ndcg3': run['metric']['retrieval_metrics']['ndcg3'],
                     'cost_usd': run['cost_microunits'] / 1e6, 'runtime': run['runtime_metrics'],
                     'code': data['experiment']['code'], 'task_id': run['task_id'], 'accounting': run['accounting'],
                     'evidence_directory': run_root.name, 'export_sha256': sha(run_root / (run['label'] + '.json')),
                     'question_workers': run.get('question_workers', manifest['question_workers']),
                     'empty_outputs': sum(not q['generated_text'].strip() for q in data['questions'])})
    assert len(runs) == 4 and sum(r['questions'] for r in runs) == 200
    browser = read(root / 'browser/status.json'); assert browser['status'] == 'passed'
    with (root / 'browser/browser-export.csv').open(encoding='utf-8-sig', newline='') as stream:
        csv_run = next(r for r in csv.DictReader(stream) if r['record_type'] == 'run')
    assert json.loads(csv_run['runtime_metrics_json']) == read(root / 'browser/browser-export.json')['runtime_metrics']
    personal = read(root / 'personal-verification.json'); assert personal['status'] == 'passed'
    budget = read(args.baseline / 'budget.json')
    paid = sum(Decimal(r.get('actual_usd', '0')) for r in budget['requests'])
    held = sum(Decimal(r['reserved_usd']) for r in budget['requests'] if 'actual_usd' not in r)
    assert paid + held < 20
    quality = read(root / 'data/quality.json')
    facts = {'data_quality': quality, 'reader': summaries, 'reader_plan': plan, 'http': runs, 'wiki': wiki,
             'browser': browser, 'personal_build': personal['system_info']['data'],
             'budget': {'limit_usd': 20, 'reported_usd': str(paid), 'unsettled_reserved_usd': str(held), 'requests': len(budget['requests'])},
             'human_rating': None}
    write(out / 'summary.json', facts)
    (out / 'README.md').write_text(render_report(facts), encoding='utf-8', newline='\n')
    for name in ['01-evaluation-overview.png', '02-question-evidence.png', '04-model-statistics.png']:
        shutil.copy2(root / 'browser' / name, out / name)
    shutil.copy2(root / 'data/sources.json', out / 'sources.json')
    write(out / 'artifact-sha256.json', {p.name: sha(p) for p in sorted(out.iterdir()) if p.is_file() and p.name != 'artifact-sha256.json'})
    print(json.dumps({'status': 'passed', 'reader_outputs': 1200, 'http_outputs': 200, 'wiki_pairs': 90, 'reported_usd': str(paid)}))


if __name__ == '__main__':
    main()
