from pathlib import Path
import concurrent.futures as cf,hashlib,json,re,subprocess,threading,time,urllib.request
root=Path('/home/gemini/wxocr-4090-20261029');out=root/'reports/concurrency8-live-20261029';cfg=(root/'examples/web/docker-nvidia/config.yaml').read_text();token=re.search(r'^token:\s*(\S+)',cfg,re.M).group(1)
config={k:re.search('^'+k+r':\s*(\S+)',cfg,re.M).group(1) for k in ['engine_workers','max_concurrent_tasks','requests_per_minute']};assert config=={'engine_workers':'8','max_concurrent_tasks':'8','requests_per_minute':'0'}
state=lambda:json.loads(subprocess.check_output(['docker','inspect','wxocr-4090-ocr-1'],text=True))[0]
deadline=time.monotonic()+150
while True:
 try:
  st=state();assert st['State']['Health']['Status']=='healthy'
  with urllib.request.urlopen('http://127.0.0.1:7676/',timeout=2):pass
  break
 except Exception:
  if time.monotonic()>deadline:raise
  time.sleep(.5)
image=(root/'testdata/accuracy-sample.jpg').read_bytes();boundary='eight-live-check';body=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="sample.jpg"\r\nContent-Type: image/jpeg\r\n\r\n').encode()+image+('\r\n--'+boundary+'--\r\n').encode()
def request(mode,barrier=None):
 if barrier:barrier.wait()
 req=urllib.request.Request('http://127.0.0.1:7676/api/ocr?mode='+mode,data=body,headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary='+boundary})
 with urllib.request.urlopen(req,timeout=120) as r:d=json.load(r)
 h=hashlib.sha256(json.dumps(d['result'],sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()).hexdigest()
 return {'mode':d['mode'],'elapsed_ms':d['elapsed_ms'],'result_sha256':h,'lines':len(d['result']['lines'])}
refs={m:request(m) for m in ['fast','high_resolution']}
barrier=threading.Barrier(8);modes=['fast','high_resolution']*4
with cf.ThreadPoolExecutor(8) as pool:results=list(pool.map(lambda m:request(m,barrier),modes))
assert all(r['result_sha256']==refs[r['mode']]['result_sha256'] for r in results)
# 72 additional successful requests exceed the former 60/minute initial quota.
small=(root/'testdata/native-cases/chinese.png').read_bytes();body=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="small.png"\r\nContent-Type: image/png\r\n\r\n').encode()+small+('\r\n--'+boundary+'--\r\n').encode()
start=time.monotonic()
with cf.ThreadPoolExecutor(8) as pool:rate_results=list(pool.map(lambda _:request('fast'),range(72)))
rate_s=time.monotonic()-start
assert len(rate_results)==72 and len({r['result_sha256'] for r in rate_results})==1
st=state();pid=str(st['State']['Pid']);apps=subprocess.check_output(['nvidia-smi','--query-compute-apps=pid,used_gpu_memory','--format=csv,noheader,nounits'],text=True)
memory=[int(r.split(',')[1]) for r in apps.splitlines() if r.split(',')[0].strip()==pid]
report={'config':config,'mixed_eight_requests':results,'quota_check_success':72,'quota_check_wall_s':rate_s,'all_results_exact':True,'image':st['Config']['Image'],'health':st['State']['Health']['Status'],'restart_count':st['RestartCount'],'oom_killed':st['State']['OOMKilled'],'system_memory_limit_bytes':st['HostConfig']['Memory'],'cpu_limits':{k:st['HostConfig'][k] for k in ['NanoCpus','CpuQuota','CpusetCpus']},'app_gpu_memory_mib':memory,'stats':json.loads(subprocess.check_output(['docker','stats','--no-stream','--format','{{json .}}','wxocr-4090-ocr-1'],text=True))}
(out/'verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2));print(json.dumps(report,ensure_ascii=False),flush=True)
