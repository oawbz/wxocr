import json,re,time,subprocess,urllib.request,urllib.error,concurrent.futures as cf,sys
from pathlib import Path
root=Path.cwd();out=root/'reports/modes-20261029';active=Path('/home/gemini/wxocr-4090-20261029');deployed=len(sys.argv)>1 and sys.argv[1]=='deployed';port=7676 if deployed else 17676
config=(active/'examples/web/docker-nvidia/config.yaml').read_text();token=re.search(r'^token:\s*(\S+)',config,re.M).group(1)
old=json.loads(Path('/home/gemini/wxocr-accuracy-20261029/reports/accuracy-deploy-20261029/candidate.json').read_text())
reference=old['sample_runs'][0]['result']
corpus={x['image']:x['result'] for x in json.loads((active/'reports/native-prepost-20261029/corpus-after.json').read_text())}
def request(name,mode=None):
 p=root/'testdata/accuracy-sample.jpg' if name=='sample' else root/'testdata/native-cases'/name
 boundary='mode-check-upload';body=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="'+p.name+'"\r\nContent-Type: '+('image/jpeg' if name=='sample' else 'image/png')+'\r\n\r\n').encode()+p.read_bytes()+('\r\n--'+boundary+'--\r\n').encode()
 url='http://127.0.0.1:'+str(port)+'/api/ocr'+('' if mode is None else '?mode='+mode)
 req=urllib.request.Request(url,data=body,headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary='+boundary})
 with urllib.request.urlopen(req,timeout=120) as response:data=json.load(response)
 assert data['mode']==('fast' if mode is None else mode)
 return data
report={};cid=None
try:
 if not deployed:
  cfg=out/'config-test.yaml';cfg.write_text(re.sub(r'^requests_per_minute:.*$','requests_per_minute: 0',config,flags=re.M));cfg.chmod(0o600)
  cid=subprocess.check_output(['docker','run','--rm','-d','--user','1000:1000','--gpus','"device=1"','--memory','4g','--read-only','--tmpfs','/tmp:size=64m,mode=1777','--cap-drop=ALL','--security-opt','no-new-privileges:true','-e','LD_LIBRARY_PATH=/opt/ort:/usr/local/cuda/lib64:/usr/lib/x86_64-linux-gnu','-p','127.0.0.1:17676:7676','-v',str(cfg)+':/app/config.yaml:ro','wxocr-web:cuda-modes-20261029'],text=True).strip()
 deadline=time.monotonic()+120
 while True:
  try:
   with urllib.request.urlopen('http://127.0.0.1:'+str(port)+'/',timeout=1) as response:page=response.read()
   break
  except Exception:
   if time.monotonic()>deadline:raise
   time.sleep(.2)
 assert page==(root/'examples/web/index.html').read_bytes()
 assert b'<option value="fast" selected>' in page
 req=urllib.request.Request('http://127.0.0.1:'+str(port)+'/api/ocr?mode=invalid',data=b'',headers={'Authorization':'Bearer '+token})
 try:urllib.request.urlopen(req);raise AssertionError('invalid mode accepted')
 except urllib.error.HTTPError as e:assert e.code==400 and json.load(e)['error']['code']=='invalid_mode'
 default=request('sample');fast=request('sample','fast');high=request('sample','high_resolution')
 assert default['result']==fast['result'],'default differs from fast'
 assert high['result']==reference,'high resolution changed previous accuracy result'
 assert all(d['pass']==0 and not d.get('retry_method') and d['reason']!='split_parent' for d in fast['result']['diagnostics'])
 mixed=['high_resolution','fast','high_resolution','fast']
 with cf.ThreadPoolExecutor(2) as pool:responses=list(pool.map(lambda mode:request('sample',mode),mixed))
 for mode,res in zip(mixed,responses):assert res['result']==(fast if mode=='fast' else high)['result'],'concurrent mode leak'
 names=sorted(corpus);modes=['fast'] if deployed else ['fast','high_resolution'];checks=[]
 for mode in modes:
  with cf.ThreadPoolExecutor(2) as pool:results=list(pool.map(lambda name:request(name,mode),names))
  for name,res in zip(names,results):
   parity=all(res['result'].get(k)==corpus[name].get(k) for k in ['width','height','lines','paragraphs'])
   checks.append({'image':name,'mode':mode,'previous_gpu_main_exact':parity,'elapsed_ms':res['elapsed_ms']})
   if mode=='high_resolution':assert parity,(name,'high resolution regression')
   if mode=='fast':assert all(d['pass']==0 for d in res['result'].get('diagnostics',[]))
 state=json.loads(subprocess.check_output(['docker','inspect',cid or 'wxocr-4090-ocr-1'],text=True))[0]
 assert state['Config']['Image']=='wxocr-web:cuda-modes-20261029'
 report={'image_id':state['Image'],'image_tag':state['Config']['Image'],'restarts':state['RestartCount'],'sample':{'default_ms':default['elapsed_ms'],'fast_ms':fast['elapsed_ms'],'high_resolution_ms':high['elapsed_ms'],'fast_lines':len(fast['result']['lines']),'high_resolution_lines':len(high['result']['lines']),'default_equals_fast':True,'high_resolution_equals_previous':True},'mixed_modes':[{'mode':m,'elapsed_ms':r['elapsed_ms']} for m,r in zip(mixed,responses)],'corpus':checks,'page_matches_source':True,'invalid_mode_status':400,'config':{k:re.search('^'+k+r':\s*(\S+)',config,re.M).group(1) for k in ['engine_workers','max_concurrent_tasks','requests_per_minute','startup_warmup','model_resident']}}
 (out/('deployed.json' if deployed else 'candidate.json')).write_text(json.dumps(report,ensure_ascii=False,indent=2))
 print(json.dumps({k:v for k,v in report.items() if k!='corpus'},ensure_ascii=False),flush=True);print('corpus',len(checks),'differences',[x for x in checks if not x['previous_gpu_main_exact']],flush=True)
finally:
 if cid:
  (out/'startup.log').write_text(subprocess.check_output(['docker','logs',cid],stderr=subprocess.STDOUT,text=True))
  subprocess.run(['docker','stop','-t','30',cid],stdout=subprocess.DEVNULL,check=False)
