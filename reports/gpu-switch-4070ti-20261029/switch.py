"""Authorized production GPU migration; rollback GPU mapping on failed acceptance."""
import json,subprocess,time,re,collections,concurrent.futures as cf,urllib.request,hashlib
from pathlib import Path
ROOT=Path('/home/gemini/wxocr-4090-20261029');C=ROOT/'examples/web/docker-nvidia';H=ROOT/'reports/gpu-switch-4070ti-20261029';H.mkdir(parents=True,exist_ok=True)
GPU='GPU-eefd2b51-d4d8-4290-af35-4121fb84dbc5';NAME='wxocr-4090-ocr-1'
def cmd(args,timeout=30):return subprocess.check_output(args,text=True,stderr=subprocess.STDOUT,cwd=C,timeout=timeout).strip()
def inspect():return json.loads(cmd(['docker','inspect',NAME]))[0]
def state(d):return {'image_tag':d['Config']['Image'],'image_id':d['Image'],'pid':d['State']['Pid'],'health':d['State'].get('Health',{}).get('Status'),'running':d['State']['Running'],'restarts':d['RestartCount'],'devices':d['HostConfig']['DeviceRequests'],'nano_cpus':d['HostConfig']['NanoCpus'],'memory_limit_bytes':d['HostConfig']['Memory']}
def save(): (H/'deployment.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
def up():return cmd(['docker','compose','up','-d','--no-build','--pull','never','ocr'],180)
def wait():
 start=time.monotonic()
 while time.monotonic()-start<180:
  d=inspect()
  if not d['State']['Running']:raise RuntimeError('container exited during startup')
  if d['State'].get('Health',{}).get('Status')=='healthy':return d
  time.sleep(2)
 raise RuntimeError('health readiness timed out')
before=inspect();env=C/'.env';oldenv=env.read_bytes() if env.exists() else None
report={'before':state(before),'target_gpu_uuid':GPU,'status':'preparing','env_existed':oldenv is not None,'config_sha256_before':hashlib.sha256((C/'config.yaml').read_bytes()).hexdigest()};save()
assert before['State'].get('Health',{}).get('Status')=='healthy'
assert cmd(['docker','image','inspect',before['Config']['Image'],'--format','{{.Id}}'])==before['Image']
if oldenv is not None:(H/'env.before').write_bytes(oldenv);(H/'env.before').chmod(0o600)
try:
 text=oldenv.decode() if oldenv else ''
 text=re.sub(r'^OCR_GPU_ID=.*\n?','',text,flags=re.M).rstrip()+'\nOCR_GPU_ID='+GPU+'\n'
 env.write_text(text.lstrip('\n'));env.chmod(0o600)
 cmd(['docker','compose','config','--quiet'])
 report['status']='recreating';save();report['compose_output']=up();after=wait()
 assert after['Image']==before['Image']
 assert after['HostConfig']['NanoCpus']==before['HostConfig']['NanoCpus']
 assert after['HostConfig']['Memory']==before['HostConfig']['Memory']
 assert after['HostConfig']['DeviceRequests'][0]['DeviceIDs']==[GPU]
 visible=cmd(['docker','exec',NAME,'nvidia-smi','--query-gpu=uuid,name','--format=csv,noheader']);assert GPU in visible
 cfg=(C/'config.yaml').read_text();token=re.search(r'^token:\s*(\S+)',cfg,re.M).group(1)
 settings={k:int(re.search(r'^'+k+r':\s*(\d+)',cfg,re.M).group(1)) for k in ['engine_workers','max_concurrent_tasks','requests_per_minute']}
 assert settings['engine_workers']==8 and settings['max_concurrent_tasks']==8
 references=json.loads(Path('/home/gemini/ppocr6-test-20261029/gpu-compare-4070ti/accuracy.json').read_text())
 cases=[('chinese.png','fast'),('dense_invoice.png','fast'),('layout_two_columns.png','fast'),('accuracy-sample.jpg','high_resolution')]*2
 def request(case):
  name,mode=case;p=ROOT/'testdata'/name if name.endswith('.jpg') else ROOT/'testdata/native-cases'/name;b='gpu-switch-acceptance'
  body=('--'+b+'\r\nContent-Disposition: form-data; name="image"; filename="'+name+'"\r\nContent-Type: application/octet-stream\r\n\r\n').encode()+p.read_bytes()+('\r\n--'+b+'--\r\n').encode()
  t=time.perf_counter();req=urllib.request.Request('http://127.0.0.1:7676/api/ocr?mode='+mode,data=body,headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary='+b})
  with urllib.request.urlopen(req,timeout=120) as res:code=res.status;data=json.load(res)
  old=references[name+'@'+mode]['result'];new=data['result']
  same=collections.Counter(l['text'] for l in old['lines'])==collections.Counter(l['text'] for l in new['lines'])
  return {'image':name,'mode':mode,'http_status':code,'wall_ms':(time.perf_counter()-t)*1000,'accepted_text_multiset_matches_4070_reference':same,'result':new}
 with cf.ThreadPoolExecutor(8) as pool:results=list(pool.map(request,cases))
 assert all(r['http_status']==200 and r['accepted_text_multiset_matches_4070_reference'] for r in results)
 (H/'acceptance-raw.json').write_text(json.dumps(results,ensure_ascii=False,indent=2))
 report.update(status='deployed',after=state(inspect()),visible_device=visible,settings=settings,acceptance_requests=[{k:v for k,v in r.items() if k!='result'} for r in results],gpu_state=cmd(['nvidia-smi','--query-gpu=index,name,memory.used,memory.total,utilization.gpu','--format=csv,noheader']),config_sha256_after=hashlib.sha256((C/'config.yaml').read_bytes()).hexdigest())
 assert report['config_sha256_after']==report['config_sha256_before'];save();print('DEPLOYED',json.dumps({k:v for k,v in report.items() if k not in ['compose_output']},ensure_ascii=False),flush=True)
except Exception as e:
 report['error']=str(e);report['status']='rolling_back';save()
 if oldenv is None:env.unlink(missing_ok=True)
 else:env.write_bytes(oldenv);env.chmod(0o600)
 try:report['rollback_output']=up();report['rollback_state']=state(wait());report['status']='rolled_back'
 except Exception as rollback:report['rollback_error']=str(rollback);report['status']='rollback_failed'
 save();print(json.dumps(report,ensure_ascii=False),flush=True);raise
