import concurrent.futures as cf
import json,re,time,subprocess,urllib.request,statistics,sys
from pathlib import Path
root=Path.cwd();out=root/'reports/accuracy-deploy-20261029';active=Path('/home/gemini/wxocr-4090-20261029')
source=(active/'examples/web/docker-nvidia/config.yaml').read_text();token=re.search(r'^token:\s*(\S+)',source,re.M).group(1)
source=re.sub(r'^requests_per_minute:.*$','requests_per_minute: 0',source,flags=re.M)
mode=sys.argv[1] if len(sys.argv)>1 else 'candidate'
port=7676 if mode=='deployed' else 17676
reference=json.loads((out/'sample-reference.json').read_text())
corpus_before={x['image']:x['result'] for x in json.loads((active/'reports/native-prepost-20261029/corpus-after.json').read_text())}
def request(name):
 p=root/'testdata'/name if name=='accuracy-sample.jpg' else root/'testdata/native-cases'/name
 image=p.read_bytes();boundary='accuracy-verify-upload'
 body=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="'+name+'"\r\nContent-Type: '+('image/jpeg' if name.endswith('.jpg') else 'image/png')+'\r\n\r\n').encode()+image+('\r\n--'+boundary+'--\r\n').encode()
 req=urllib.request.Request('http://127.0.0.1:'+str(port)+'/api/ocr',data=body,headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary='+boundary})
 start=time.monotonic()
 with urllib.request.urlopen(req,timeout=120) as r:data=json.load(r)
 return {'image':name,'wall_ms':(time.monotonic()-start)*1000,'elapsed_ms':data['elapsed_ms'],'result':data['result']}
def upright(line,result):return line['top'],result['width']-line['right'],line['bottom'],result['width']-line['left']
def metrics(result):
 checks={}
 for column,bounds in reference['columns'].items():
  rows=[[] for _ in range(10)]
  for line in result['lines']:
   x0,y0,x1,y1=upright(line,result);x,y=(x0+x1)/2,(y0+y1)/2
   if not bounds[0]<x<bounds[1]:continue
   for row in range(10):
    if reference['edges'][row]<=y<reference['edges'][row+1]:rows[row].append((y0,x0,line['text']));break
  values=[''.join(t for _,__,t in sorted(row)) for row in rows]
  normalize=(lambda s:re.sub(r'\D','',s)) if column in ['birth','work','retirement'] else (lambda s:re.sub(r'\s','',s))
  checks[column]={'complete':sum(normalize(v)==e for v,e in zip(values,reference['expected'][column])),'texts':values}
 texts=[x['text'] for x in result['lines']];observed=set(re.findall(r'[0-9]{15,20}[Xx]?','\n'.join(texts)))
 footer=[x for x in result['lines'] if (upright(x,result)[1]+upright(x,result)[3])/2>2200]
 account=[x['text'] for x in footer if 1780<(upright(x,result)[0]+upright(x,result)[2])/2<2000]
 ids=[x['text'] for x in footer if re.fullmatch(r'[:：\s]*[0-9]{15,20}[Xx]?',x['text'])]
 return {'lines':len(texts),'candidates':len(result.get('candidates',[])),'diagnostics':len(result.get('diagnostics',[])),'local_passes':max([x['pass'] for x in result.get('diagnostics',[])],default=0),'checks':checks,'source_id_strings_present':sum(x in observed for x in reference['ids']),'footer_independent_ids':len(ids),'footer_account_correct':sum(t=='2005-01' for t in account),'review_fields':[{'text':x.get('text'),'reasons':x['review_reasons']} for x in result.get('diagnostics',[]) if x['reason']=='accepted' and any(r!='inferred_table_column' for r in x.get('review_reasons',[]))]}
report={'mode':mode};cid=None
try:
 if mode!='deployed':
  cfg=out/(mode+'-config.yaml');cfg.write_text(source);cfg.chmod(0o600)
  tag='wxocr-web:cuda-native-20261029' if mode=='before' else 'wxocr-web:cuda-accuracy-20261029'
  start=time.monotonic();cid=subprocess.check_output(['docker','run','--rm','-d','--user','1000:1000','--gpus','"device=1"','--read-only','--tmpfs','/tmp:size=64m,mode=1777','--cap-drop=ALL','--security-opt','no-new-privileges:true','--memory','4g','--pids-limit','256','-e','LD_LIBRARY_PATH=/opt/ort:/usr/local/cuda/lib64:/usr/lib/x86_64-linux-gnu','-p','127.0.0.1:17676:7676','-v',str(cfg)+':/app/config.yaml:ro',tag],text=True).strip()
  while True:
   try:
    with urllib.request.urlopen('http://127.0.0.1:17676/',timeout=1):pass
    break
   except Exception:
    if time.monotonic()-start>120:raise RuntimeError('startup timeout')
    time.sleep(.2)
  report['ready_ms']=(time.monotonic()-start)*1000
 request('accuracy-sample.jpg')
 report['sample_runs']=[request('accuracy-sample.jpg') for _ in range(3)]
 report['median_elapsed_ms']=statistics.median(x['elapsed_ms'] for x in report['sample_runs'])
 report['sample_metrics']=metrics(report['sample_runs'][0]['result'])
 assert all(x['result']==report['sample_runs'][0]['result'] for x in report['sample_runs']),'nondeterministic sample'
 if mode!='before':
  m=report['sample_metrics'];assert all(x['complete']==10 for x in m['checks'].values()),m
  assert m['source_id_strings_present']==13 and m['footer_independent_ids']==3 and m['footer_account_correct']==3,m
  assert m['review_fields'] and m['local_passes']==8,m
  with cf.ThreadPoolExecutor(2) as pool:
   corpus=list(pool.map(request,sorted(corpus_before)))
  report['corpus']=[{'image':x['image'],'elapsed_ms':x['elapsed_ms'],'main_result_exact':all(x['result'].get(k)==corpus_before[x['image']].get(k) for k in ['width','height','lines','paragraphs'])} for x in corpus]
  assert all(x['main_result_exact'] for x in report['corpus']),report['corpus']
  with cf.ThreadPoolExecutor(2) as pool:parallel=list(pool.map(request,['accuracy-sample.jpg']*2))
  assert all(x['result']==report['sample_runs'][0]['result'] for x in parallel),'parallel sample mismatch'
  report['parallel_sample_ms']=[x['elapsed_ms'] for x in parallel]
  if mode=='deployed':
   candidate=json.loads((out/'candidate.json').read_text())
   assert candidate['sample_runs'][0]['result']==report['sample_runs'][0]['result'],'deployment differs from candidate'
 container=cid or 'wxocr-4090-ocr-1';state=json.loads(subprocess.check_output(['docker','inspect',container],text=True))[0]
 report['image_tag']=state['Config']['Image'];report['image_id']=state['Image'];report['restarts']=state['RestartCount']
 pid=str(state['State']['Pid']);apps=subprocess.check_output(['nvidia-smi','--query-compute-apps=pid,used_gpu_memory','--format=csv,noheader,nounits'],text=True)
 report['gpu_memory_mib']=[int(row.split(',')[1]) for row in apps.splitlines() if row.split(',')[0].strip()==pid]
 report['container']=json.loads(subprocess.check_output(['docker','stats','--no-stream','--format','{{json .}}',container],text=True))
 (out/(mode+'.json')).write_text(json.dumps(report,ensure_ascii=False,indent=2))
 print(mode,'median_ms',report['median_elapsed_ms'],'metrics',json.dumps(report['sample_metrics'],ensure_ascii=False),'GPU_MiB',report['gpu_memory_mib'],'corpus',len(report.get('corpus',[])),flush=True)
finally:
 if cid:
  (out/(mode+'-startup.log')).write_text(subprocess.check_output(['docker','logs',cid],stderr=subprocess.STDOUT,text=True))
  subprocess.run(['docker','stop','-t','30',cid],stdout=subprocess.DEVNULL,check=False)
