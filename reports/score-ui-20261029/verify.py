from pathlib import Path
import concurrent.futures as cf,hashlib,json,re,subprocess,time,urllib.request,sys
root=Path.cwd();out=root/'reports/score-ui-20261029';active=Path('/home/gemini/wxocr-4090-20261029');deployed=len(sys.argv)>1 and sys.argv[1]=='deployed';port=7676 if deployed else 17676
cfg=(active/'examples/web/docker-nvidia/config.yaml').read_text();token=re.search(r'^token:\s*(\S+)',cfg,re.M).group(1)
old=json.loads((active/'reports/concurrency8-live-20261029/verification.json').read_text());expected={r['mode']:r['result_sha256'] for r in old['mixed_eight_requests']}
boundary='score-ui-verify';image=(root/'testdata/accuracy-sample.jpg').read_bytes();body=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="sample.jpg"\r\nContent-Type: image/jpeg\r\n\r\n').encode()+image+('\r\n--'+boundary+'--\r\n').encode()
def request(mode):
 req=urllib.request.Request('http://127.0.0.1:'+str(port)+'/api/ocr?mode='+mode,data=body,headers={'Authorization':'Bearer '+token,'Content-Type':'multipart/form-data; boundary='+boundary})
 with urllib.request.urlopen(req,timeout=120) as response:data=json.load(response)
 result=data['result'];h=hashlib.sha256(json.dumps(result,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()).hexdigest();assert h==expected[mode]
 return {'mode':mode,'elapsed_ms':data['elapsed_ms'],'result_sha256':h,'body_lines':len(result['lines']),'display_frames':len(result['lines'])+len(result.get('candidates',[]))}
cid=None
try:
 if not deployed:
  config=out/'candidate-config.yaml';text=cfg
  for k in ['engine_workers','max_concurrent_tasks']:text=re.sub('^'+k+r':.*$',k+': 2',text,flags=re.M)
  config.write_text(text);config.chmod(0o600)
  cid=subprocess.check_output(['docker','run','--rm','-d','--user','1000:1000','--gpus','"device=1"','--memory','4g','--read-only','--tmpfs','/tmp:size=64m,mode=1777','--cap-drop=ALL','--security-opt','no-new-privileges:true','-e','LD_LIBRARY_PATH=/opt/ort:/usr/local/cuda/lib64:/usr/lib/x86_64-linux-gnu','-p','127.0.0.1:17676:7676','-v',str(config)+':/app/config.yaml:ro','wxocr-web:cuda-score-ui-20261029'],text=True).strip()
 deadline=time.monotonic()+150
 while True:
  try:
   with urllib.request.urlopen('http://127.0.0.1:'+str(port)+'/',timeout=1) as response:page=response.read()
   assert page==(root/'examples/web/index.html').read_bytes()
   break
  except Exception:
   if time.monotonic()>deadline:raise
   time.sleep(.2)
 assert b'recall-summary' not in page and b'function displayLines' in page and b'confidenceText(line.confidence,2)' in page
 modes=['fast','high_resolution']*(4 if deployed else 2)
 with cf.ThreadPoolExecutor(8 if deployed else 2) as pool:results=list(pool.map(request,modes))
 state=json.loads(subprocess.check_output(['docker','inspect',cid or 'wxocr-4090-ocr-1'],text=True))[0];assert state['Config']['Image']=='wxocr-web:cuda-score-ui-20261029'
 report={'page_matches_source':True,'separate_candidate_ui_removed':True,'full_ocr_results_exact_to_previous':True,'requests':results,'image_tag':state['Config']['Image'],'image_id':state['Image'],'restarts':state['RestartCount'],'config':{k:re.search('^'+k+r':\s*(\S+)',cfg,re.M).group(1) for k in ['engine_workers','max_concurrent_tasks','requests_per_minute','model_resident','startup_warmup']}}
 (out/('deployed.json' if deployed else 'candidate.json')).write_text(json.dumps(report,ensure_ascii=False,indent=2));print(json.dumps(report,ensure_ascii=False),flush=True)
finally:
 if cid:subprocess.run(['docker','stop','-t','30',cid],stdout=subprocess.DEVNULL,check=False)
