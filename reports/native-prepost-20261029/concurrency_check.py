"""Run isolated HTTP workers on GPU 1; never export the server token."""
import concurrent.futures as cf
import json,re,time,subprocess,urllib.request,threading,statistics
from pathlib import Path
root=Path.cwd(); out=root/'reports/native-prepost-20261029'
source=(root/'examples/web/docker-nvidia/config.yaml').read_text()
token=re.search(r'^token:\s*(\S+)',source,re.M).group(1)
source=re.sub(r'^(engine_workers|max_concurrent_tasks|requests_per_minute|startup_warmup):.*\n','',source,flags=re.M)
baseline={x['image']:x['result'] for x in json.loads((root/'reports/compact-ctc-20261029/corpus-after.json').read_text())}
images={p.name:p.read_bytes() for p in (root/'testdata/native-cases').glob('*.png') if p.name in baseline}
report={'tests':[],'corpus':[],'note':'GPU utilization/power is whole GPU shared with other processes; GPU process memory and container CPU/RAM are application-specific. No TFLOPS estimate.'}
def command(args):return subprocess.check_output(args,text=True,stderr=subprocess.STDOUT).strip()
def request(name):
 boundary='native-pool-measure'
 body=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="'+name+'"\r\nContent-Type: image/png\r\n\r\n').encode()+images[name]+('\r\n--'+boundary+'--\r\n').encode()
 req=urllib.request.Request('http://127.0.0.1:17676/api/ocr',data=body,headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary='+boundary})
 start=time.monotonic()
 with urllib.request.urlopen(req,timeout=100) as r:data=json.load(r)
 assert data['result']==baseline[name], 'Result mismatch: '+name
 return {'image':name,'wall_ms':(time.monotonic()-start)*1000,'elapsed_ms':data['elapsed_ms']}
def sample(cid,pid):
 procs=command(['nvidia-smi','--query-compute-apps=pid,used_gpu_memory','--format=csv,noheader,nounits'])
 memory=[int(row.split(',')[1].strip()) for row in procs.splitlines() if row.split(',')[0].strip()==str(pid)]
 gpu=command(['nvidia-smi','-i','1','--query-gpu=utilization.gpu,utilization.memory,power.draw,memory.used','--format=csv,noheader,nounits'])
 stats=json.loads(command(['docker','stats','--no-stream','--format','{{json .}}',cid]))
 return {'time':time.time(),'app_gpu_memory_mib':memory,'whole_gpu':gpu,'container':stats}
for mode,tag,workers,clients in [('before','wxocr-web:cuda-compact-20261029',None,1),('native1','wxocr-web:cuda-native-20261029',1,1),('native1','wxocr-web:cuda-native-20261029',1,2),('native2','wxocr-web:cuda-native-20261029',2,1),('native2','wxocr-web:cuda-native-20261029',2,2)]:
 cfg=out/(mode+'.yaml');cfg.write_text(source+'\nmax_concurrent_tasks: 2\nrequests_per_minute: 0\nstartup_warmup: true\n'+('' if workers is None else 'engine_workers: '+str(workers)+'\n'));cfg.chmod(0o600)
 cid=None;stop=threading.Event();samples=[];sampler=None
 try:
  start=time.monotonic()
  cid=command(['docker','run','--rm','-d','--user','1000:1000','--gpus','"device=1"','--read-only','--tmpfs','/tmp:size=64m,mode=1777','--cap-drop=ALL','--security-opt','no-new-privileges:true','--memory','4g','--pids-limit','256','-e','LD_LIBRARY_PATH=/opt/ort:/usr/local/cuda/lib64:/usr/lib/x86_64-linux-gnu','-p','127.0.0.1:17676:7676','-v',str(cfg)+':/app/config.yaml:ro',tag])
  while True:
   try:
    with urllib.request.urlopen('http://127.0.0.1:17676/',timeout=1):pass
    break
   except Exception:
    if time.monotonic()-start>120:raise RuntimeError(command(['docker','logs',cid])[-3000:])
    time.sleep(.1)
  ready_ms=(time.monotonic()-start)*1000;pid=int(command(['docker','inspect','-f','{{.State.Pid}}',cid]))
  idle=sample(cid,pid)
  if workers==2 and clients==2:
   with cf.ThreadPoolExecutor(2) as pool:
    report['corpus']=list(pool.map(request,sorted(images)))
  def monitoring():
   while not stop.is_set():
    try:samples.append(sample(cid,pid))
    except Exception as e:samples.append({'error':str(e)})
    stop.wait(.25)
  sampler=threading.Thread(target=monitoring);sampler.start()
  names=['chinese.png','dense_invoice.png','layout_two_columns.png']*20
  start=time.monotonic()
  with cf.ThreadPoolExecutor(clients) as pool:
   pending=[pool.submit(request,n) for n in names]
   pmon=command(['nvidia-smi','pmon','-i','1','-s','um','-d','1','-c','2'])
   results=[f.result() for f in pending]
  wall=time.monotonic()-start;stop.set();sampler.join();sampler=None
  latency=sorted(x['wall_ms'] for x in results)
  test={'mode':mode,'workers':workers or 1,'clients':clients,'count':len(results),'ready_ms':ready_ms,'wall_s':wall,'requests_per_second':len(results)/wall,'p50_ms':statistics.median(latency),'p95_ms':latency[int(len(latency)*.95)-1],'idle':idle,'samples':samples,'pmon':pmon,'results':results}
  report['tests'].append(test);(out/'concurrency.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
  (out/(mode+'-c'+str(clients)+'-startup.log')).write_text(command(['docker','logs',cid]))
  print(mode,clients,'rps',round(test['requests_per_second'],2),'p50',round(test['p50_ms']),'p95',round(test['p95_ms']),'idleGPU',idle['app_gpu_memory_mib'],flush=True)
 finally:
  stop.set()
  if sampler:sampler.join()
  if cid:subprocess.run(['docker','stop','-t','20',cid],stdout=subprocess.DEVNULL,check=False)
print('ALL_HTTP_RESULTS_EXACT',len(report['corpus']),flush=True)
