import collections, hashlib, json, math, random, shutil
from decimal import Decimal, ROUND_HALF_UP
from pathlib import Path
import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

import argparse
parser=argparse.ArgumentParser(description='Render frozen acceptance evidence; requires numpy and matplotlib.')
parser.add_argument('--evidence',type=Path,required=True)
parser.add_argument('--output',type=Path)
args=parser.parse_args()
ROOT=args.evidence.resolve()
REPO=Path(__file__).resolve().parents[1]
OUT=args.output or REPO/'docs/reports/final-acceptance'
OUT.mkdir(parents=True,exist_ok=True)
def read(p): return json.loads(p.read_text(encoding='utf-8-sig'))
def write(p,d): p.write_text(json.dumps(d,ensure_ascii=False,indent=2),encoding='utf-8')
manifest=read(ROOT/'live-04/manifest.json');assert manifest['status']=='passed'
probe_status=read(ROOT/'probes-01/status.json');assert probe_status['status']=='passed'
judge_status=read(ROOT/'judge-02/status.json');assert judge_status['judged_pairs']==48
personal=read(ROOT/'personal-demo/result.json');assert personal['status']=='passed'
fault=read(ROOT/'fault-01/fault-results.json');assert fault['status']=='passed'
judgments=read(ROOT/'judge-02/judgments.json')
receipts=[json.loads(line) for line in (ROOT/'judge-02/supplier.jsonl').read_text(encoding='utf-8').splitlines()]
assert len(receipts)==len(judgments)==48
for row,receipt in zip(judgments,receipts):
    assert receipt['status']==200 and len(row['ledger'])==1
    cost=int((Decimal(str(receipt['usage']['cost']))*1000000).quantize(Decimal(1),rounding=ROUND_HALF_UP))
    assert row['ledger'][0]['cost_microunits']==cost
budget=read(ROOT/'budget.json')
paid=sum(Decimal(str(x.get('actual_usd',0))) for x in budget['requests'])
held=sum(Decimal(str(x['reserved_usd'])) for x in budget['requests'] if 'actual_usd' not in x)
assert paid+held<=20
formal=[];review=[]
for r in manifest['rounds']:
    if r['label'].startswith('cache-'):continue
    export=read(ROOT/'live-04'/(r['label']+'.json'))
    ledger=read(ROOT/'live-04'/(r['label']+'-ledger.json'))
    dataset=r['label'].split('-')[0];qs=export['questions']
    source='cmrc2018-dev' if dataset=='cmrc' else 'squad2-dev'
    passages=sorted(read(REPO/f'dataset/public/{source}/v1/registry-input.json')['passages'],key=lambda p:p['pid'])
    answerable=[q for q in qs if q['ground_truth_pids']]
    votes=[]
    for row in judgments:
        if row['dataset']!=dataset or row['status']!='valid':continue
        key=next(k for k,v in row['identities'].items() if v==r['model'])
        votes.append(row['judgment'][key])
    item={'dataset':dataset,'model':r['model'],'task_id':r['task_id'],'questions':len(qs),
          'answerable':len(answerable),'unanswerable':len(qs)-len(answerable),
          'recall_all':r['metric']['retrieval_metrics']['recall'],
          'recall_answerable':float(np.mean([q['per_sample_metrics']['retrieval_metrics']['recall'] for q in answerable])),
          'rougel':r['metric']['generation_metrics']['rougel'],
          'p50_ms':float(np.percentile([q['total_ms'] for q in qs],50)),
          'p95_ms':float(np.percentile([q['total_ms'] for q in qs],95)),
          'chat_cost_usd':sum(x['cost_microunits'] for x in ledger if x['operation']=='chat')/1e6,
          'total_cost_usd':r['cost_microunits']/1e6,
          'judge_valid':len(votes),'judge_correct':sum(x['correct'] for x in votes),
          'judge_supported':sum(x['supported'] for x in votes),
          'code':export['experiment']['code'],'dataset_identity':export['experiment']['dataset']}
    formal.append(item)
    for q in qs:
        vote=next(x for x in judgments if x['dataset']==dataset and x['qid']==q['qid'])
        key=next(k for k,v in vote['identities'].items() if v==r['model'])
        review.append({'dataset':dataset,'model':r['model'],'qid':q['qid'],'question':q['question'],
            'reference_answer':q['reference_answer'],'generated_text':q['generated_text'],
            'ground_truth_pids':q['ground_truth_pids'],'generation_pids':q['generation_pids'],
            'evidence':[{'index':pid,'pid':passages[pid]['pid'],'content':passages[pid]['content']} for pid in q['generation_pids']],
            'model_assisted_review':vote.get('judgment',{}).get(key),
            'human_rating':None,'human_reviewer':None,'human_notes':None})
assert len(formal)==4 and len(review)==96
cache=[]
for pair in range(1,6):
    item={'pair':pair}
    for arm in ('cold','warm'):
        label=f'cache-{pair}-{arm}'
        rows=read(ROOT/'live-04'/(label+'-ledger.json'))
        embeddings=[r for r in rows if r['operation']=='embedding']
        item[arm+'_calls']=len(embeddings)
        item[arm+'_embedding_usd']=sum(r['cost_microunits'] for r in embeddings)/1e6
    assert item['cold_calls']==9 and item['warm_calls']==0
    cache.append(item)
probes=read(ROOT/'probes-01/results.json')
wiki=[]
for pair in range(1,31):
    item={'pair':pair}
    for arm in ('stable-prefix','page-first'):
        row=next(r for r in probes if r['step']==f'wiki-{pair:02d}-{arm}')
        ledger=row['ledger'][0]
        item[arm]={'cache_read_tokens':ledger.get('provider_cache_read_tokens'),
                   'prompt_tokens':ledger['prompt_tokens'],'cost_usd':ledger['cost_microunits']/1e6,
                   'fact_checks':row['fact_checks'],'answer':row['result']}
    wiki.append(item)
deltas=[r['stable-prefix']['cache_read_tokens']-r['page-first']['cache_read_tokens'] for r in wiki
        if r['stable-prefix']['cache_read_tokens'] is not None and r['page-first']['cache_read_tokens'] is not None]
assert deltas
rng=random.Random(1729)
boot=[sum(rng.choices(deltas,k=len(deltas)))/len(deltas) for _ in range(5000)]
ci=[float(x) for x in np.percentile(boot,[2.5,97.5])]
wiki_summary={'pairs':30,'cache_reported_pairs':len(deltas),'mean_cache_read_delta':float(np.mean(deltas)),
 'paired_bootstrap_95ci':ci,'bootstrap_seed':1729,'bootstrap_iterations':5000,
 'outcome':'observed_positive_difference' if ci[0]>0 else 'inconclusive_or_nonpositive'}
for arm in ('stable-prefix','page-first'):
    wiki_summary[arm]={'cache_read_tokens':sum((r[arm]['cache_read_tokens'] or 0) for r in wiki),
      'prompt_tokens':sum(r[arm]['prompt_tokens'] for r in wiki),
      'fact_checks_passed':sum(all(r[arm]['fact_checks'].values()) for r in wiki),
      'total_cost_usd':sum(r[arm]['cost_usd'] for r in wiki)}
summary={'formal_runs':formal,'cache_pairs':cache,'wiki':wiki_summary,
 'budget':{'limit_usd':20,'reported_actual_usd':str(paid),'unsettled_reserved_usd':str(held),'requests_or_reserved_batches':len(budget['requests'])},
 'fault_cases':[{'case':r['case'],'status':r['detail']['task']['status']} for r in fault['cases']],
 'human_review_status':'pending','personal_demo_status':personal['status'],
 'formal_binary_sha256':manifest['server_sha256'],
 'formal_runner_sha256':hashlib.sha256((ROOT/'live-04/runner-source.py').read_bytes()).hexdigest()}
write(OUT/'summary.json',summary);write(OUT/'human-review.json',review);write(OUT/'wiki-pairs.json',wiki)
plt.rcParams.update({'font.family':'Microsoft YaHei','axes.spines.top':False,'axes.spines.right':False,
 'axes.labelcolor':'#253746','text.color':'#253746','axes.edgecolor':'#c7d2dc','font.size':10,'figure.facecolor':'white'})
colors=['#166b8f','#da8b37'];labels=['中文 / DeepSeek','中文 / Kimi','英文 / DeepSeek','英文 / Kimi']
fig,axs=plt.subplots(1,3,figsize=(13,4.5),layout='constrained')
for ax,title,values in ((axs[0],'有答案问题：召回率',[r['recall_answerable'] for r in formal]),
 (axs[1],'独立模型复核：正确率',[r['judge_correct']/r['judge_valid'] for r in formal]),
 (axs[2],'独立模型复核：证据支持率',[r['judge_supported']/r['judge_valid'] for r in formal])):
    ax.bar(range(4),values,color=colors*2,width=.65)
    ax.set_xticks(range(4),labels,rotation=22,ha='right');ax.set_ylim(0,1.13);ax.set_title(title,pad=16)
    ax.set_yticks([0,.25,.5,.75,1],['0%','25%','50%','75%','100%']);ax.grid(axis='y',alpha=.18);ax.set_axisbelow(True)
    for i,v in enumerate(values):ax.text(i,v+.025,f'{v:.1%}',ha='center',fontsize=11)
fig.suptitle('固定公开子集 · 48 个问题 × 2 个回答模型',fontsize=15,fontweight='bold')
fig.savefig(OUT/'quality.png',dpi=200);plt.close(fig)
fig,axs=plt.subplots(1,2,figsize=(12,4.6),layout='constrained')
x=np.arange(1,6);axs[0].bar(x-.18,[r['cold_calls'] for r in cache],.36,label='冷启动',color=colors[0]);axs[0].bar(x+.18,[r['warm_calls'] for r in cache],.36,label='重启后复用',color=colors[1])
axs[0].set(title='持久化向量缓存：每组 2 题 / 32 段落',xlabel='独立冷 / 热配对序号',ylabel='向量供应商请求次数',xticks=x,ylim=(0,10.5));axs[0].legend(frameon=False);axs[0].grid(axis='y',alpha=.18);axs[0].set_axisbelow(True)
for r in cache:axs[0].text(r['pair']+.18,.25,'0',ha='center',color=colors[1])
for row in wiki:
    values=[row[a]['cache_read_tokens'] for a in ('page-first','stable-prefix')]
    if None not in values:axs[1].plot([0,1],values,color='#7d9caf',alpha=.28,marker='o',markersize=3)
means=[np.mean([r[a]['cache_read_tokens'] for r in wiki if r[a]['cache_read_tokens'] is not None]) for a in ('page-first','stable-prefix')]
axs[1].plot([0,1],means,color='#164d64',linewidth=3,marker='o',markersize=8,label='均值')
axs[1].set(title='Wiki 前缀：30 组配对，固定 GMICloud 路由',xticks=[0,1],xticklabels=['页面变量在前','共享资料在前'],ylabel='供应商报告的缓存读取 token');axs[1].grid(axis='y',alpha=.18);axs[1].legend(frameon=False)
fig.savefig(OUT/'cache.png',dpi=200);plt.close(fig)
budget_models=collections.defaultdict(Decimal)
for r in budget['requests']:budget_models[r['model']]+=Decimal(str(r.get('actual_usd',0)))
fig,ax=plt.subplots(figsize=(9,4.3),layout='constrained');names=list(budget_models);vals=[float(budget_models[n]) for n in names]
ax.barh(names,vals,color='#166b8f');ax.invert_yaxis();ax.set(xlabel='供应商报告费用（美元）',title=f'累计确认 ${paid:.6f} · 保守预留 ${held:.6f} · 预算 $20')
ax.grid(axis='x',alpha=.18);ax.set_axisbelow(True);fig.savefig(OUT/'cost.png',dpi=200);plt.close(fig)
quality_rows='\n'.join(f"| {r['dataset']} | {r['model'].split('/')[-1]} | {r['questions']} | {r['recall_answerable']:.3f} | {r['rougel']:.3f} | {r['judge_correct']}/{r['judge_valid']} | {r['judge_supported']}/{r['judge_valid']} | {r['chat_cost_usd']:.6f} |" for r in formal)
wiki_conclusion=('固定前缀组观察到更高的供应商缓存读取量。' if ci[0]>0 else '配对区间包含零或负值，当前样本没有确立固定前缀的正向收益。')
report=f'''# WeKnora 课题三验收报告

系统提供固定数据评测、模型调用账本、持久化向量缓存与回归门禁。真实实验包含 96 份公开数据回答、五组服务重启缓存对照、30 组 Wiki 前缀对照和 48 组匿名答案自动复核。个人服务保留可打开的合成资料知识库与两题评测记录。

## 1. 运行与数据流

个人服务由 PostgreSQL 数据库、Redis 缓存和队列、DocReader 文档解析、WeKnora 后端及网页组成。应用程序编程接口（Application Programming Interface，API）通过本机端口 18080 提供服务，网页入口为 `http://127.0.0.1:5174`。双击项目外层的 `启动WeKnora.cmd` 启动完整服务。

评测任务固定数据版本、模型配置、分块策略、检索参数及指标算法版本。每次供应商调用先写入起始记录，收到结果后保存用量、费用与终态。OpenRouter 报告的十进制美元费用按四舍五入转换为微美元，同时保留原始金额和价格估算值。服务意外终止时，未确定的费用继续保持未确定状态。

## 2. 真实回答质量

中文阅读理解数据集 CMRC2018（Chinese Machine Reading Comprehension 2018）与斯坦福问答数据集 SQuAD2（Stanford Question Answering Dataset 2.0）各使用 24 个问题、32 个候选段落的固定开发集子集。SQuAD2 子集包含 16 个有答案问题和 8 个无答案问题。两个回答模型使用相同数据、向量模型与检索配置；生成温度为 0，输出上限为 512 个 token。

图 1 展示有答案问题的召回率和独立模型复核结果。召回率衡量相关段落是否进入检索结果；自动复核分别判断回答正确性与事实是否获得所提供段落支持。

![图 1：检索与自动复核](quality.png)

图中自动复核由 DeepSeek V4 Pro 执行，答案匿名化为 A、B，并按题目交替交换顺序。模型辅助评分与人工评分分别保存。单个自动评分者可能存在判断误差及模型家族偏好，人工评分字段保持空值。有效评分覆盖 {sum(r['status']=='valid' for r in judgments)}/48 组；格式无效的评分保留原始输出并排除出比例的分母。

下表补充答案词序重合指标 ROUGE-L（Recall-Oriented Understudy for Gisting Evaluation，基于最长公共子序列的摘要相似度）和对话调用费用。

| 数据 | 回答模型 | 题数 | 有答案召回率 | ROUGE-L | 自动正确 | 自动证据支持 | 对话费用 / USD |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
{quality_rows}

参考答案通常为短文本，回答中的扩展解释与引用标记会影响词面指标。无答案问题的空参考答案也限制词面相似度的解释范围。质量判断同时保留逐题回答、参考答案、检索段落标识和自动复核理由，见 `human-review.json`。这些子集结果适用于本次固定候选段落范围。

评分一致性检查发现题目 `cmrc2018-dev-DEV_121_QUERY_4` 的 Kimi 答案被标为不正确，但评分理由同时确认其日期符合参考答案。该机器分值按原始回执保留，列为人工复核项。自动评分比例描述评分器输出，不能单独作为回答质量的定论。

无答案问题暴露了错误前提识别与证据边界问题。两个模型在 SQuAD2 的 8 个无答案问题上均获得 4 个自动正确判断。题目 `5ad4d8d65b96ef001a10a363` 将资料中的 Palace on the Water 改为 Palace on the Bank，两个模型均接受替换后的名称并给出日期；题目 `5ad158c0645df0001a2d1829` 中 DeepSeek 将资料中的服务获取增加表述为减少；题目 `5ad3ade2604f3c001a3fec18` 中 Kimi 将资料未明确列出的命令边界展开为具体断言。完整问题标识带 `squad2-dev-` 前缀，原始证据与回答见逐题文件。检索命中相关资料后，生成阶段仍需要核对问题前提和每项结论的证据。

## 3. 缓存实测

图 2 左侧展示五组完整服务重启对照，右侧展示 Wiki 页面更新的供应商缓存读取量。向量缓存实验每组使用 32 个固定段落和 2 个问题，冷启动前仅清理隔离数据库中的向量缓存；热启动前结束并重新启动后端进程。

![图 2：持久化向量缓存和 Wiki 前缀配对](cache.png)

五组向量请求均从 9 次降为 0 次，逐题检索结果保持一致。内容变化探测中，相同内容跨进程复用缓存，修改一条输入后仅该条输入到达供应商。对话生成费用受输出长度和实际路由影响，向量请求减少与整轮费用变化分别记录。

Wiki 实验调用生产页面提示词，在相同资料和参数下移动共享资料块的位置，固定 GMICloud 路由，交替两组请求顺序。{wiki_conclusion} 每对请求的缓存读取量平均差为 {wiki_summary['mean_cache_read_delta']:.1f} 个 token，配对自助法 95% 区间为 [{ci[0]:.1f}, {ci[1]:.1f}]，使用 5,000 次重采样和固定随机种子 1729。有效缓存报告覆盖 {len(deltas)}/30 对。事实检查的规则命中结果和全文保存在 `wiki-pairs.json`。

这些顺序请求共享供应商缓存环境。区间描述本批次配对差值的重采样分布，跨请求缓存复用会形成相关性；独立部署、其他路由及其他资料长度下的收益需要对应实验。

## 4. 费用与故障证据

图 3 按模型展示预算文件中的供应商报告费用。目录报价与实际路由扣费分别保存，失去明确费用回执的请求保留完整预留金额。

![图 3：模型费用](cost.png)

本次新增预算上限为 20 美元，确认费用为 {paid} 美元，未确定费用的保守预留为 {held} 美元。确认费用与保守预留之和为 {paid+held} 美元。此前独立授权的小额冒烟测试不计入该新增预算。Qwen 共享上游的 HTTP 429 限流记录作为可用性失败证据保留；正式质量对照采用成功完成的 DeepSeek 和 Kimi 批次。

接口取消、重复取消和进程强制终止均通过隔离服务验证。取消任务发布状态 6，执行租约到期后的恢复任务发布状态 5。故障测试使用固定供应商响应，付费请求为零。个人服务中的模型价格页、统计接口、健康接口和 12 个入口静态资源通过检查；两题合成资料评测在实际 PostgreSQL 服务中完成。

## 5. 持续集成与工程复现

持续集成（Continuous Integration，CI）的必需检查为 `Deterministic golden evaluation`。主分支启用严格状态检查并适用于管理员。[功能验收检查](https://github.com/Aokiuuz/WeKnora/actions/runs/34348192656)在提交 `97a8dad0` 上通过。受控退化实验将召回率从 0.75 降为 0.375，[失败检查](https://github.com/Aokiuuz/WeKnora/actions/runs/34344846357)报告 `retrieval.recall` 退化并以失败退出，合并状态为 `BLOCKED`。受控实验拉取请求（Pull Request，PR）已关闭，正式交付使用 [PR #1](https://github.com/Aokiuuz/WeKnora/pull/1)。

[Go 静态检查](https://github.com/Aokiuuz/WeKnora/actions/runs/34348192517)在提交 `97a8dad0` 上报告 475 项问题并失败，涉及主模块、命令行和客户端。其中 472 项所在文件与集成基线 `1776aa44` 完全相同；另外三项位于客户端模型声明及探测程序。当前源码包含这三项的注释和格式修复。全部静态检查通过仍是仓库工程质量未完成项，结果与功能验收分别记录。

Apple M4 工程证据对应提交 `1776aa442a491f1657196ca21da914bab197b17c`。证据包记录两个独立运行、988 项对照通过，固定指标与 Linux 参考值一致，付费请求为零。真实模型实验的执行二进制安全散列算法 256 位（Secure Hash Algorithm 256-bit，SHA-256）为 `{manifest['server_sha256']}`，执行脚本副本摘要为 `{summary['formal_runner_sha256']}`。真实实验导出的代码提交字段为 unknown，审计依赖上述二进制、脚本和数据身份；该字段按实际记录保留。

## 6. 验收入口

运行操作见 [个人服务说明](../../personal-service.md)，模型角色、价格口径与命令见 [OpenRouter 验收说明](../../openrouter-acceptance.md)，逐步展示见 [演示与交付索引](../../acceptance-demo.md)。`summary.json` 保存汇总数值，`human-review.json` 保存 96 份回答、证据段落和模型辅助意见，`wiki-pairs.json` 保存 30 组配对数据。人工评分需要真实评分者填写；本报告中的自动评分均保留机器评分身份。浏览器视觉交互尚未完成独立验证。
'''
(OUT/'README.md').write_text(report,encoding='utf-8')
write(OUT/'artifact-sha256.json',{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in OUT.iterdir() if p.is_file() and p.name!='artifact-sha256.json'})
print(json.dumps({'report':str(OUT/'README.md'),'reported_actual_usd':str(paid),'held_usd':str(held)},ensure_ascii=False))

