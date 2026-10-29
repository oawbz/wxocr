import concurrent.futures as cf
import json,re,time,subprocess,urllib.request,urllib.error,threading
from pathlib import Path
root=Path.cwd();out=root/'reports/native-prepost-20261029'
cfg=(root/'examples/web/docker-nvidia/config.yaml').read_text()
token=re.search(r'^token:\s*(\S+)',cfg,re.M).group(1)
baseline={x['image']:x['result'] for x in json.loads((root/'reports/compact-ctc-20261029/corpus-after.json').read_text())}
def request(name,barrier=None):
 boundary='native-deploy-check';image=(root/'testdata/native-cases'/name).read_bytes()
 body=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="'+name+'"\r\nContent-Type: image/png\r\n\r\n').encode()+image+('\r\n--'+boundary+'--\r\n').encode()
 req=urllib.request.Request('http://127.0.0.1:7676/api/ocr',data=body,headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary='+boundary})
 if barrier:barrier.wait()
 start=time.monotonic()
 try:
  with urllib.request.urlopen(req,timeout=100) as r:data=json.load(r)
  assert data['result']==baseline[name],name+' mismatch'
  return {'image':name,'status':200,'elapsed_ms':data['elapsed_ms'],'wall_ms':(time.monotonic()-start)*1000,'exact':True}
 except urllib.error.HTTPError as e:
  return {'image':name,'status':e.code,'error':json.load(e),'retry_after':e.headers.get('Retry-After')}
with cf.ThreadPoolExecutor(2) as pool:corpus=list(pool.map(request,sorted(baseline)))
assert all(x['status']==200 for x in corpus),corpus
barrier=threading.Barrier(8)
with cf.ThreadPoolExecutor(8) as pool:burst=list(pool.map(lambda _:request('dense_invoice.png',barrier),range(8)))
assert any(x['status']==429 for x in burst),burst
assert any(x['status']==200 for x in burst),burst
assert all(x['status'] in (200,429) for x in burst),burst
with urllib.request.urlopen('http://127.0.0.1:7676/',timeout=5) as r:page=r.status
try:urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:7676/api/ocr',data=b''),timeout=5);raise AssertionError('missing auth accepted')
except urllib.error.HTTPError as e:assert e.code==401
cid='wxocr-4090-ocr-1'
state=json.loads(subprocess.check_output(['docker','inspect',cid],text=True))[0]
pid=state['State']['Pid']
apps=subprocess.check_output(['nvidia-smi','--query-compute-apps=pid,used_gpu_memory','--format=csv,noheader,nounits'],text=True)
mem=[int(r.split(',')[1]) for r in apps.splitlines() if r.split(',')[0].strip()==str(pid)]
report={'corpus':corpus,'burst':burst,'page_status':page,'unauthenticated_status':401,'image_id':state['Image'],'image_tag':state['Config']['Image'],'restarts':state['RestartCount'],'gpu_memory_mib':mem,'container':json.loads(subprocess.check_output(['docker','stats','--no-stream','--format','{{json .}}',cid],text=True)),'config':{k:re.search('^'+k+r':\s*(\S+)',cfg,re.M).group(1) for k in ['engine_workers','max_concurrent_tasks','requests_per_minute','model_resident','startup_warmup']}}
(out/'deployment.json').write_text(json.dumps(report,ensure_ascii=False,indent=2));print(json.dumps({k:v for k,v in report.items() if k not in ['corpus','burst','container']},ensure_ascii=False));print('corpus_exact',len(corpus),'burst_statuses',[x['status'] for x in burst])
