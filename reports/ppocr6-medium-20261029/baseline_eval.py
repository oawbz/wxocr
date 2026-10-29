"""Closed-loop 2/8-engine pressure tests on the deployed image; private token stays remote."""
import concurrent.futures as cf
import hashlib,json,re,statistics,subprocess,threading,time,urllib.request,urllib.error
from pathlib import Path
root=Path.cwd();out=root/'baseline';out.mkdir(exist_ok=True);active=Path('/home/gemini/wxocr-4090-20261029')
source=(active/'examples/web/docker-nvidia/config.yaml').read_text();token=re.search(r'^token:\s*(\S+)',source,re.M).group(1)
source=re.sub(r'^(engine_workers|max_concurrent_tasks|requests_per_minute):.*\n','',source,flags=re.M)
tag='wxocr-web:cuda-fragment-20261029';duration=30
payloads={}
for name in [x['file'] for x in json.loads((root/'truth.json').read_text())['cases']]+['accuracy-sample.jpg']:
 p=active/'testdata'/name if name.endswith('.jpg') else active/'testdata/native-cases'/name
 boundary='concurrency-eight-benchmark'
 payloads[name]=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="'+name+'"\r\nContent-Type: '+('image/jpeg' if name.endswith('.jpg') else 'image/png')+'\r\n\r\n').encode()+p.read_bytes()+('\r\n--'+boundary+'--\r\n').encode()
headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary=concurrency-eight-benchmark'}
references={};reference_results={};report={'image_tag':tag,'duration_target_s':duration,'method':'Closed-loop client concurrency equals independent engine workers, 30 seconds per workload plus drain; 4GiB container, PID limit256, startup warmup enabled, rate limit0 in isolated localhost container; no previews; every full result checked against the same-image two-worker warm reference. Production remains unchanged.','tests':[],'startup':[]}
def command(args):return subprocess.check_output(args,text=True,stderr=subprocess.STDOUT,timeout=12).strip()
def digest(result):return hashlib.sha256(json.dumps(result,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()).hexdigest()
def request(name,mode):
 start=time.perf_counter();status=0
 try:
  req=urllib.request.Request('http://127.0.0.1:17676/api/ocr?mode='+mode,data=payloads[name],headers=headers)
  with urllib.request.urlopen(req,timeout=120) as response:status=response.status;data=json.load(response)
  result=data['result'];key=(name,mode);h=digest(result)
  if key not in references:references[key]=h;reference_results[key]=result
  exact=h==references[key]
  summary=None
  if not exact:
   old=reference_results[key];summary={'lines_equal':result.get('lines')==old.get('lines'),'line_texts_equal':[x['text'] for x in result['lines']]==[x['text'] for x in old['lines']],'paragraphs_equal':result.get('paragraphs')==old.get('paragraphs')}
  return {'status':status,'image':name,'mode':mode,'wall_ms':(time.perf_counter()-start)*1000,'elapsed_ms':data['elapsed_ms'],'exact':exact,'mismatch':summary}
 except urllib.error.HTTPError as e:return {'status':e.code,'wall_ms':(time.perf_counter()-start)*1000,'error':e.read().decode()[:500]}
 except Exception as e:return {'status':status,'wall_ms':(time.perf_counter()-start)*1000,'error':str(e)[:500]}
def sample(cid,pid):
 processes=command(['nvidia-smi','--query-compute-apps=pid,used_gpu_memory','--format=csv,noheader,nounits'])
 memory=[int(r.split(',')[1]) for r in processes.splitlines() if r.split(',')[0].strip()==str(pid)]
 pmon=command(['nvidia-smi','pmon','-i','1','-s','um','-d','1','-c','1']);sm=None;memory_util=None
 for row in pmon.splitlines():
  c=row.split()
  if len(c)>9 and c[1]==str(pid):
   sm=int(c[3]) if c[3]!='-' else None;memory_util=int(c[4]) if c[4]!='-' else None
 stats=json.loads(command(['docker','stats','--no-stream','--format','{{json .}}',cid]))
 whole=command(['nvidia-smi','-i','1','--query-gpu=utilization.gpu,power.draw,memory.used','--format=csv,noheader,nounits'])
 return {'time':time.time(),'app_gpu_memory_mib':memory,'app_sm_percent':sm,'app_memory_util_percent':memory_util,'container':stats,'whole_gpu_util_power_memory':whole}
def save():
 (out/'results.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
workloads=[('small_fast',['chinese.png','dense_invoice.png','layout_two_columns.png'],'fast'),('large_fast',['accuracy-sample.jpg'],'fast'),('large_high_resolution',['accuracy-sample.jpg'],'high_resolution')]
for workers in [8]:
 cid=None;sampler=None;stop=threading.Event()
 try:
  cfg=out/('config-'+str(workers)+'.yaml');cfg.write_text(source+'\nengine_workers: '+str(workers)+'\nmax_concurrent_tasks: '+str(workers)+'\nrequests_per_minute: 0\n');cfg.chmod(0o600)
  start=time.monotonic();cid=command(['docker','run','--rm','-d','--user','1000:1000','--gpus','"device=1"','--read-only','--tmpfs','/tmp:size=64m,mode=1777','--memory','4g','--pids-limit','256','--cap-drop=ALL','--security-opt','no-new-privileges:true','-e','LD_LIBRARY_PATH=/opt/ort:/usr/local/cuda/lib64:/usr/lib/x86_64-linux-gnu','-p','127.0.0.1:17676:7676','-v',str(cfg)+':/app/config.yaml:ro',tag])
  while True:
   try:
    with urllib.request.urlopen('http://127.0.0.1:17676/',timeout=1):pass
    break
   except Exception:
    state=json.loads(command(['docker','inspect',cid]))[0]['State']
    if not state['Running'] or time.monotonic()-start>150:raise RuntimeError('container not ready: '+command(['docker','logs',cid])[-2000:])
    time.sleep(.2)
  state=json.loads(command(['docker','inspect',cid]))[0];pid=state['State']['Pid']
  report['startup'].append({'workers':workers,'ready_ms':(time.monotonic()-start)*1000,'image_id':state['Image'],'idle':sample(cid,pid)})
  accuracy={}
  for mode in ['fast','high_resolution']:
   for name in payloads:
    trials=[request(name,mode) for _ in range(3)]
    accuracy[name+'@'+mode]={'trials':trials,'result':reference_results.get((name,mode))}
    (out/'accuracy.json').write_text(json.dumps(accuracy,ensure_ascii=False,indent=2))
  print('ACCURACY_COMPLETE',len(accuracy),flush=True)
  # Establish exactly one reference per input/mode before spawning pressure threads.
  for _,names,mode in workloads:
   for name in names:
    warm=request(name,mode);assert warm['status']==200 and warm['exact'],warm
  for workload,names,mode in workloads:
   samples=[];stop=threading.Event()
   def monitoring():
    while not stop.is_set():
     try:samples.append(sample(cid,pid))
     except Exception as e:samples.append({'time':time.time(),'error':str(e)[:300]})
     stop.wait(.2)
   sampler=threading.Thread(target=monitoring);sampler.start()
   barrier=threading.Barrier(workers+1);deadline=[0.0];abort=threading.Event()
   def client(index):
    barrier.wait();rows=[];i=index
    while time.perf_counter()<deadline[0] and not abort.is_set():
     row=request(names[i%len(names)],mode);rows.append(row);i+=1
     if row['status']!=200:
      time.sleep(.2)
      if len(rows)>=3 and all(x['status']!=200 for x in rows[-3:]):abort.set()
    return rows
   with cf.ThreadPoolExecutor(workers) as pool:
    futures=[pool.submit(client,i) for i in range(workers)]
    start=time.perf_counter();deadline[0]=start+duration;barrier.wait()
    rows=[r for f in futures for r in f.result()];wall=time.perf_counter()-start
   stop.set();sampler.join();sampler=None
   success=[r for r in rows if r['status']==200 and 'exact' in r];latencies=sorted(r['wall_ms'] for r in success)
   test={'workers':workers,'clients':workers,'workload':workload,'mode':mode,'requests':len(rows),'success':len(success),'failures':len(rows)-len(success),'result_mismatches':sum(not r['exact'] for r in success),'wall_s':wall,'requests_per_second':len(success)/wall,'p50_ms':statistics.median(latencies) if latencies else None,'p95_ms':latencies[max(0,int(len(latencies)*.95)-1)] if latencies else None,'max_ms':max(latencies,default=None),'samples':samples,'requests_detail':rows}
   report['tests'].append(test);save()
   print('workers',workers,workload,'requests',len(rows),'fail',test['failures'],'mismatch',test['result_mismatches'],'rps',round(test['requests_per_second'],2),'p95',round(test['p95_ms'] or 0),flush=True)
   if abort.is_set():raise RuntimeError('repeated request failures; pressure stopped')
  (out/('startup-'+str(workers)+'.log')).write_text(command(['docker','logs',cid]))
 except Exception as e:
  report.setdefault('errors',[]).append({'workers':workers,'error':str(e)[:3000]});save();print('ERROR',workers,str(e),flush=True)
 finally:
  stop.set()
  if sampler:sampler.join()
  if cid:
   try:
    state=json.loads(command(['docker','inspect',cid]))[0]
    report.setdefault('final_states',[]).append({'workers':workers,'state':state['State'],'restart_count':state['RestartCount']});save()
    subprocess.run(['docker','stop','-t','30',cid],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=45)
   except Exception as e:print('cleanup',str(e),flush=True)
report['production_state']=command(['docker','inspect','-f','{{.Config.Image}} {{.State.Health.Status}} {{.RestartCount}}','wxocr-4090-ocr-1'])
save();print('BENCHMARK_COMPLETE',report['production_state'],flush=True)
