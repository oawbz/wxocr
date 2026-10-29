import json,statistics,re
from pathlib import Path
out=Path('reports/concurrency8-20261029');raw=json.loads((out/'results.json').read_text())
def memory_mib(value):
 number,unit=re.match(r'([0-9.]+)\s*(\w+)',value).groups();return float(number)*{'B':1/1048576,'KiB':1/1024,'MiB':1,'GiB':1024}[unit]
s={'date':'2026-10-29','method':raw['method'],'image_tag':raw['image_tag'],'duration_target_s':raw['duration_target_s'],'startup':raw['startup'],'errors':raw.get('errors',[]),'tests':[],'production_state':raw.get('production_state'),'resources_note':'App GPU memory and SM are per process; CPU and RAM are per container. Observed sampled peaks are not hard maxima. Whole-GPU metrics include other services. Docker CPU100% equals one logical core.'}
for t in raw['tests']:
 samples=[x for x in t['samples'] if 'container' in x];sm=[x['app_sm_percent'] for x in samples if x['app_sm_percent'] is not None]
 r={k:v for k,v in t.items() if k not in ['samples','requests_detail']}
 r['resources']={'sample_count':len(samples),'app_gpu_memory_peak_mib':max([m for x in samples for m in x['app_gpu_memory_mib']],default=0),'app_sm_mean_percent':statistics.mean(sm) if sm else None,'app_sm_max_percent':max(sm,default=None),'container_cpu_peak_percent':max([float(x['container']['CPUPerc'].rstrip('%')) for x in samples],default=0),'container_memory_peak_mib':max([memory_mib(x['container']['MemUsage']) for x in samples],default=0)}
 r['error_examples']=[x for x in t['requests_detail'] if x['status']!=200 or not x.get('exact',False)][:3]
 s['tests'].append(r)
s['final_states']=[{'workers':x['workers'],'running':x['state']['Running'],'oom_killed':x['state']['OOMKilled'],'restart_count':x['restart_count']} for x in raw.get('final_states',[])]
(out/'summary.json').write_text(json.dumps(s,ensure_ascii=False,indent=2))
for t in s['tests']:print(t['workers'],t['workload'],'rps',round(t['requests_per_second'],2),'p95',round(t['p95_ms'] or 0),'fail',t['failures'],'mismatch',t['result_mismatches'],t['resources'])
