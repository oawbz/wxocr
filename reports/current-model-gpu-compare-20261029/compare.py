"""Compare current-model output and isolated one-core/one-client GPU runs."""
import json,re,statistics,collections
from pathlib import Path
from text_score import score
H=Path(__file__).resolve().parent
truth=json.loads((H/'truth.json').read_text())['cases']
raw={g:json.loads((H/('gpu-compare-'+g)/'accuracy.json').read_text()) for g in ['4070ti','4090']}
report={g:json.loads((H/('gpu-compare-'+g)/'results.json').read_text()) for g in raw}
assert raw['4070ti'].keys()==raw['4090'].keys()
cross=[]
for key,a in raw['4070ti'].items():
 b=raw['4090'][key];x=a['result'];y=b['result']
 assert x is not None and y is not None
 assert all(r['status']==200 and r['exact'] for r in a['trials']+b['trials'])
 xt=[r['text'] for r in x['lines']];yt=[r['text'] for r in y['lines']]
 candidates_equal=[r['text'] for r in x.get('candidates',[])]==[r['text'] for r in y.get('candidates',[])]
 pairs=[];remaining=list(y['lines'])
 if collections.Counter(xt)==collections.Counter(yt):
  for line in x['lines']:
   choices=[(i,q) for i,q in enumerate(remaining) if q['text']==line['text']]
   idx,q=min(choices,key=lambda v:sum((line[c]-v[1][c])**2 for c in ['left','top','right','bottom']))
   pairs.append((line,q));remaining.pop(idx)
 confidence_delta=max((abs(p['confidence']-q['confidence']) for p,q in pairs),default=0) if len(pairs)==len(x['lines']) else None
 bbox_delta=max((abs(p[c]-q[c]) for p,q in pairs for c in ['left','top','right','bottom']),default=0) if len(pairs)==len(x['lines']) else None
 def paragraphs(r):return [[r['lines'][i]['text'] for i in p['line_indices']] for p in r.get('paragraphs',[])]
 cross.append(dict(case=key,full_result_equal=x==y,accepted_texts_equal=xt==yt,accepted_text_multiset_equal=collections.Counter(xt)==collections.Counter(yt),candidate_texts_equal=candidates_equal,candidate_text_multiset_equal=collections.Counter(r['text'] for r in x.get('candidates',[]))==collections.Counter(r['text'] for r in y.get('candidates',[])),paragraph_text_groups_equal=paragraphs(x)==paragraphs(y),max_confidence_delta=confidence_delta,max_bbox_coordinate_delta=bbox_delta,paragraphs_4070=paragraphs(x) if paragraphs(x)!=paragraphs(y) else None,paragraphs_4090=paragraphs(y) if paragraphs(x)!=paragraphs(y) else None))

def mib(s):
 n,u=re.match(r'([0-9.]+)([A-Za-z]+)',s.split('/')[0].strip()).groups();return float(n)*{'MiB':1,'GiB':1024,'KiB':1/1024,'B':1/1048576}[u]
out={'conditions':{'resident_engines':1,'clients':1,'cpu_quota_cores':1,'GOMAXPROCS':1,'ram_limit_gib':4,'load_duration_s':30,'production_changed':False},'gpus':{},'cross_gpu':{'cases':len(cross),'full_result_equal_cases':sum(r['full_result_equal'] for r in cross),'accepted_text_equal_cases':sum(r['accepted_texts_equal'] for r in cross),'candidate_text_equal_cases':sum(r['candidate_texts_equal'] for r in cross),'accepted_text_multiset_equal_cases':sum(r['accepted_text_multiset_equal'] for r in cross),'candidate_text_multiset_equal_cases':sum(r['candidate_text_multiset_equal'] for r in cross),'paragraph_text_groups_equal_cases':sum(r['paragraph_text_groups_equal'] for r in cross),'details':cross}}
for g,r in report.items():
 assert all(t['failures']==0 and t['result_mismatches']==0 for t in r['tests'])
 ss=[s for t in r['tests'] for s in t['samples']]+[s['idle'] for s in r['startup']]
 accuracy={}
 for mode in ['fast','high_resolution']:
  scores={t['file']:score(t['lines'],[l['text'] for l in raw[g][t['file']+'@'+mode]['result']['lines']]) for t in truth}
  totals={k:sum(v[k] for v in scores.values()) for k in ['reference_characters','character_errors','reference_lines','exact_lines']}
  accuracy[mode]={'totals':totals,'cases':scores}
 out['gpus'][g]={'visible_device':r['device_visible_in_container'],'image_id':r['startup'][0]['image_id'],'config_sha256':r['runtime_config_sha256'],'warmup_ms':r['startup'][0]['ready_ms'],'app_gpu_peak_mib':max(sum(s.get('app_gpu_memory_mib',[])) for s in ss),'container_ram_peak_mib':max(mib(s['container']['MemUsage']) for s in ss),'cpu_peak_percent':max(float(s['container']['CPUPerc'].strip('%')) for s in ss),'app_sm_mean_percent':(statistics.mean(s['app_sm_percent'] for s in ss if s.get('app_sm_percent') is not None) if any(s.get('app_sm_percent') is not None for s in ss) else None),'accuracy':accuracy,'tests':[{k:t[k] for k in ['workload','requests','success','failures','result_mismatches','wall_s','requests_per_second','p50_ms','p95_ms']} for t in r['tests']]}
assert out['gpus']['4070ti']['image_id']==out['gpus']['4090']['image_id']
assert out['gpus']['4070ti']['config_sha256']==out['gpus']['4090']['config_sha256']
out['timing_comparison']=[]
for a,b in zip(out['gpus']['4070ti']['tests'],out['gpus']['4090']['tests']):
 assert a['workload']==b['workload']
 out['timing_comparison'].append({'workload':a['workload'],'4070ti_p50_ms':a['p50_ms'],'4090_p50_ms':b['p50_ms'],'4070ti_latency_delta_percent':(a['p50_ms']/b['p50_ms']-1)*100,'4070ti_throughput_delta_percent':(a['requests_per_second']/b['requests_per_second']-1)*100})
(H/'summary.json').write_text(json.dumps(out,ensure_ascii=False,indent=2))
print(json.dumps({'cross_gpu':{k:v for k,v in out['cross_gpu'].items() if k!='details'},'timing_comparison':out['timing_comparison'],'resources':{k:{s:v[s] for s in ['app_gpu_peak_mib','container_ram_peak_mib','cpu_peak_percent','app_sm_mean_percent']} for k,v in out['gpus'].items()}},ensure_ascii=False,indent=2))
