import json,re,statistics
from pathlib import Path
H=Path(__file__).resolve().parent
accuracy=json.loads((H/'accuracy-summary.json').read_text());table=json.loads((H/'table-summary.json').read_text())
def mem(s):
 n,u=re.match(r'([0-9.]+)([A-Za-z]+)',s.split('/')[0].strip()).groups()
 return float(n)*{'MiB':1,'GiB':1024,'KiB':1/1024,'B':1/1048576}[u]
out={'conditions':{'clients':1,'resident_engines':1,'cpu_quota_cores':1,'container_ram_limit_gib':4,'gpu':'RTX4090, shared with unchanged existing services','workload_duration_s':15,'preview':False,'production_changed':False},'models':{}}
for label,folder,mode,scorekey in [('current_fast','baseline-single','fast','current_fast_single'),('current_high','baseline-single','high_resolution','current_high_single'),('ppocr6_default','paddle-single-default','default','ppocr6_single_default'),('ppocr6_1280','paddle-single-limit1280','limit1280','ppocr6_single_1280')]:
 x=json.loads((H/folder/'results.json').read_text());raw=json.loads((H/folder/'accuracy.json').read_text());samples=[s for t in x['tests'] for s in t['samples']]+[s['idle'] for s in x['startup']]
 tests=[t for t in x['tests'] if t['mode']==mode] if all('mode' in t for t in x['tests']) else x['tests']
 if label.startswith('current'):tests=[t for t in x['tests'] if t.get('mode')==mode]
 out['models'][label]={'accuracy':accuracy[scorekey]['totals'],'table_fields':table[scorekey],'app_gpu_peak_mib':max(sum(s.get('app_gpu_memory_mib',[])) for s in samples),'container_ram_peak_mib':max(mem(s['container']['MemUsage']) for s in samples),'cpu_peak_percent':max(float(s['container']['CPUPerc'].strip('%')) for s in samples),'table_accuracy_run_median_ms':statistics.median(r['wall_ms'] for r in raw['accuracy-sample.jpg@'+mode]['trials']),'tests':[{k:t[k] for k in ['workload','requests','success','failures','result_mismatches','wall_s','requests_per_second','p50_ms','p95_ms']} for t in tests]}
(H/'single-summary.json').write_text(json.dumps(out,ensure_ascii=False,indent=2))
print(json.dumps({k:{a:v[a] for a in ['app_gpu_peak_mib','container_ram_peak_mib','table_accuracy_run_median_ms']} for k,v in out['models'].items()},ensure_ascii=False,indent=2))
