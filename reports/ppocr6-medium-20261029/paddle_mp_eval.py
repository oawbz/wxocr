"""Closed-loop 2/8-engine pressure tests on the deployed image; private token stays remote."""
import concurrent.futures as cf
import hashlib,json,re,statistics,subprocess,threading,time,urllib.request,urllib.error
from pathlib import Path
root=Path.cwd();out=root/'paddle-mp';out.mkdir(exist_ok=True);active=Path('/home/gemini/wxocr-4090-20261029')
token='';tag='ppocr6-medium:test-20261029';duration=30
payloads={}
for name in [x['file'] for x in json.loads((root/'truth.json').read_text())['cases']]+['accuracy-sample.jpg']:
 p=active/'testdata'/name if name.endswith('.jpg') else active/'testdata/native-cases'/name
 boundary='concurrency-eight-benchmark'
 payloads[name]=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="'+name+'"\r\nContent-Type: '+('image/jpeg' if name.endswith('.jpg') else 'image/png')+'\r\n\r\n').encode()+p.read_bytes()+('\r\n--'+boundary+'--\r\n').encode()
headers={'Content-Type':'multipart/form-data; boundary=concurrency-eight-benchmark'}
references={};reference_results={};report={'image_tag':tag,'duration_target_s':duration,'method':'Closed-loop client concurrency equals independent engine workers, 30 seconds per workload plus drain; 4GiB container, PID limit256, startup warmup enabled, rate limit0 in isolated localhost container; no previews; every full result checked against the same-image two-worker warm reference. Production remains unchanged.','tests':[],'startup':[]}
report['method']='4 OS processes, two independent resident pipelines each, 8 closed-loop clients, 30 seconds plus drain; per-process Paddle GPU allocator1100MiB, cuDNN workspace64MiB; container RAM12GiB and PID512; OpenCV/OpenBLAS/OMP/MKL threads1; generic detection max long edge1280; document and textline orientation on, unwarping off. GPU memory statistics sum all child process PIDs. Production remains unchanged.'
def command(args):return subprocess.check_output(args,text=True,stderr=subprocess.STDOUT,timeout=12).strip()
def digest(result):return hashlib.sha256(json.dumps(result,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()).hexdigest()
def request(name,mode):
 start=time.perf_counter();status=0
 try:
  req=urllib.request.Request('http://127.0.0.1:17679/api/ocr?mode='+mode,data=payloads[name],headers=headers)
  with urllib.request.urlopen(req,timeout=120) as response:status=response.status;data=json.load(response)
  result=data['result'];key=(name,mode);h=digest([r['text'] for r in result['lines']])
  if key not in references:references[key]=h;reference_results[key]=result
  exact=h==references[key]
  summary=None
  if not exact:
   old=reference_results[key];summary={'lines_equal':result.get('lines')==old.get('lines'),'line_texts_equal':[x['text'] for x in result['lines']]==[x['text'] for x in old['lines']],'paragraphs_equal':result.get('paragraphs')==old.get('paragraphs')}
  return {'status':status,'image':name,'mode':mode,'wall_ms':(time.perf_counter()-start)*1000,'elapsed_ms':data['elapsed_ms'],'exact':exact,'mismatch':summary}
 except urllib.error.HTTPError as e:return {'status':e.code,'wall_ms':(time.perf_counter()-start)*1000,'error':e.read().decode()[:500]}
 except Exception as e:return {'status':status,'wall_ms':(time.perf_counter()-start)*1000,'error':str(e)[:500]}
def sample(cid,pid):
 pids={int(s.strip()) for s in command(['docker','top',cid,'-eo','pid']).splitlines() if s.strip().isdigit()}
 gpu=[]
 for row in command(['nvidia-smi','--query-compute-apps=pid,used_gpu_memory','--format=csv,noheader,nounits']).splitlines():
  c=row.split(',')
  if len(c)==2 and int(c[0]) in pids:gpu.append({'pid':int(c[0]),'memory_mib':int(c[1])})
 pmon=command(['nvidia-smi','pmon','-i','1','-s','um','-d','1','-c','1']);util=[]
 for row in pmon.splitlines():
  c=row.split()
  if len(c)>9 and c[1].isdigit() and int(c[1]) in pids:util.append({'pid':int(c[1]),'sm_percent':int(c[3]) if c[3]!='-' else None})
 stats=json.loads(command(['docker','stats','--no-stream','--format','{{json .}}',cid]))
 return {'time':time.time(),'app_gpu_memory_mib':[sum(x['memory_mib'] for x in gpu)],'gpu_processes':gpu,'process_utilization':util,'container':stats}
def save():
 (out/'results.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
workloads=[('small_limit1280',['chinese.png','dense_invoice.png','layout_two_columns.png'],'limit1280'),('large_limit1280',['accuracy-sample.jpg'],'limit1280')]

accuracy={}
for name in payloads:
 trials=[request(name,'limit1280') for _ in range(3)]
 accuracy[name+'@limit1280']={'trials':trials,'result':reference_results.get((name,'limit1280'))}
 (out/'accuracy.json').write_text(json.dumps(accuracy,ensure_ascii=False,indent=2))
print('ACCURACY_COMPLETE',len(accuracy),flush=True)
cid='ppocr6-medium-test';pid=int(command(['docker','inspect','-f','{{.State.Pid}}',cid]))
report['startup']=[{'workers':8,'idle':sample(cid,pid)}]
for workload,names,mode in workloads:
 for name in names:request(name,mode)
 samples=[];stop=threading.Event()
 def monitoring():
  while not stop.is_set():
   try:samples.append(sample(cid,pid))
   except Exception as e:samples.append({'error':str(e)})
   stop.wait(.5)
 sampler=threading.Thread(target=monitoring);sampler.start()
 barrier=threading.Barrier(9);deadline=[0.]
 def client(index):
  barrier.wait();rows=[];i=index
  while time.perf_counter()<deadline[0]:
   row=request(names[i%len(names)],mode);rows.append(row);i+=1
   if row['status']!=200:break
  return rows
 with cf.ThreadPoolExecutor(8) as pool:
  futures=[pool.submit(client,i) for i in range(8)]
  start=time.perf_counter();deadline[0]=start+duration;barrier.wait()
  rows=[r for f in futures for r in f.result()];wall=time.perf_counter()-start
 stop.set();sampler.join()
 good=[r for r in rows if r['status']==200 and 'exact' in r];latencies=sorted(r['wall_ms'] for r in good)
 test=dict(workers=8,clients=8,workload=workload,requests=len(rows),success=len(good),failures=len(rows)-len(good),result_mismatches=sum(not r['exact'] for r in good),wall_s=wall,requests_per_second=len(good)/wall,p50_ms=statistics.median(latencies) if latencies else None,p95_ms=latencies[max(0,int(len(latencies)*.95)-1)] if latencies else None,samples=samples,requests_detail=rows)
 report['tests'].append(test);save();print('TEST',workload,len(rows),test['requests_per_second'],test['p95_ms'],flush=True)
save();print('BENCHMARK_COMPLETE',flush=True)
