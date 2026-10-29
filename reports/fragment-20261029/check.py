from pathlib import Path
import concurrent.futures as cf,hashlib,json,re,subprocess,time,urllib.request,sys
root=Path.cwd();out=root/'reports/fragment-20261029';active=Path('/home/gemini/wxocr-4090-20261029');deployed=len(sys.argv)>1 and sys.argv[1]=='deployed';port=7676
cfg=(active/'examples/web/docker-nvidia/config.yaml').read_text();token=re.search(r'^token:\s*(\S+)',cfg,re.M).group(1)
old_corpus={x['image']:x['result'] for x in json.loads((active/'reports/native-prepost-20261029/corpus-after.json').read_text())}
def main(result):return {k:result.get(k) for k in ['width','height','lines','paragraphs']}
def request(name,mode):
 p=root/'testdata/accuracy-sample.jpg' if name=='sample' else root/'testdata/native-cases'/name
 boundary='generic-fragment-check';body=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="'+p.name+'"\r\nContent-Type: '+('image/jpeg' if name=='sample' else 'image/png')+'\r\n\r\n').encode()+p.read_bytes()+('\r\n--'+boundary+'--\r\n').encode()
 req=urllib.request.Request('http://127.0.0.1:'+str(port)+'/api/ocr?mode='+mode,data=body,headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary='+boundary})
 with urllib.request.urlopen(req,timeout=120) as response:d=json.load(response)
 return d
cid=None
try:
 if not deployed:
  baseline={m:request('sample',m) for m in ['fast','high_resolution']};(out/'before.json').write_text(json.dumps(baseline,ensure_ascii=False,indent=2))
  config=out/'candidate-config.yaml';text=cfg
  for k in ['engine_workers','max_concurrent_tasks']:text=re.sub('^'+k+r':.*$',k+': 2',text,flags=re.M)
  config.write_text(text);config.chmod(0o600)
  cid=subprocess.check_output(['docker','run','--rm','-d','--user','1000:1000','--gpus','"device=1"','--memory','4g','--read-only','--tmpfs','/tmp:size=64m,mode=1777','--cap-drop=ALL','--security-opt','no-new-privileges:true','-e','LD_LIBRARY_PATH=/opt/ort:/usr/local/cuda/lib64:/usr/lib/x86_64-linux-gnu','-p','127.0.0.1:17676:7676','-v',str(config)+':/app/config.yaml:ro','wxocr-web:cuda-fragment-20261029'],text=True).strip();port=17676
 else:baseline=json.loads((out/'before.json').read_text())
 deadline=time.monotonic()+150
 while True:
  try:
   with urllib.request.urlopen('http://127.0.0.1:'+str(port)+'/',timeout=1) as response:assert response.read()==(root/'examples/web/index.html').read_bytes()
   break
  except Exception:
   if time.monotonic()>deadline:raise
   time.sleep(.2)
 samples={};modes=['fast','high_resolution']*(4 if deployed else 1)
 with cf.ThreadPoolExecutor(8 if deployed else 2) as pool:responses=list(pool.map(lambda m:request('sample',m),modes))
 for mode,d in zip(modes,responses):
  before,after=baseline[mode]['result'],d['result'];
  if mode=='high_resolution':assert main(before)==main(after),'enhanced body changed'
  else:
   for line in before['lines']:
    assert any(''.join(line['text'].split()) in ''.join(other['text'].split()) and max(0,min(line['right'],other['right'])-max(line['left'],other['left']))*max(0,min(line['bottom'],other['bottom'])-max(line['top'],other['top'])) >= (line['right']-line['left'])*(line['bottom']-line['top'])*.6 for other in after['lines']),('accepted text lost',line)
  joined=[d for d in after['diagnostics'] if d.get('split_method')=='adjacent_fragments']
  assert all(d['evidence']>=.8 for d in joined)
  removed=[c for c in before.get('candidates',[]) if c not in after.get('candidates',[])]
  assert len(before['diagnostics'])<=len(after['diagnostics'])
  for old,new in zip(before['diagnostics'],after['diagnostics']):assert {k:v for k,v in old.items() if k not in ['reason','duplicate_of']}=={k:v for k,v in new.items() if k not in ['reason','duplicate_of']},'original observation lost'
  if mode=='high_resolution':assert {'07-','-01','技','位'}.issubset({c['text'] for c in removed})
  samples[mode]={'body_lines':len(after['lines']),'candidates_before':len(before.get('candidates',[])),'candidates_after':len(after.get('candidates',[])),'removed':[{'text':c['text'],'bounds':[c[k] for k in ['left','top','right','bottom']]} for c in removed],'body_exact':main(before)==main(after),'body_before':len(before['lines']),'joined':[{'text':d.get('text'),'reason':d['reason'],'evidence':d['evidence']} for d in joined],'observations_retained':True,'elapsed_ms':d['elapsed_ms']}
  if not deployed:(out/('after-'+mode+'.json')).write_text(json.dumps(d,ensure_ascii=False,indent=2))
 checks=[]
 for mode in (['fast'] if deployed else ['fast','high_resolution']):
  names=sorted(old_corpus)
  with cf.ThreadPoolExecutor(8 if deployed else 2) as pool:results=list(pool.map(lambda name:request(name,mode),names))
  for name,d in zip(names,results):assert main(d['result'])==main(old_corpus[name]),(name,mode,'corpus body changed');checks.append({'image':name,'mode':mode,'body_exact':True})
 state=json.loads(subprocess.check_output(['docker','inspect',cid or 'wxocr-4090-ocr-1'],text=True))[0]
 report={'samples':samples,'corpus':checks,'mixed_requests':len(responses),'image':state['Config']['Image'],'image_id':state['Image'],'restarts':state['RestartCount'],'config':{k:re.search('^'+k+r':\s*(\S+)',cfg,re.M).group(1) for k in ['engine_workers','max_concurrent_tasks','requests_per_minute','model_resident','startup_warmup']}}
 (out/('deployed.json' if deployed else 'candidate.json')).write_text(json.dumps(report,ensure_ascii=False,indent=2));print(json.dumps(report,ensure_ascii=False),flush=True)
finally:
 if cid:subprocess.run(['docker','stop','-t','30',cid],stdout=subprocess.DEVNULL,check=False)
