"""Evaluation-only table regions and transcribed fields. Never used in OCR."""
import json,re,unicodedata
from pathlib import Path
H=Path(__file__).resolve().parent
ref=json.loads((H/'table-truth.json').read_text())
summary={};details={}
for label,folder,mode in [('4070ti_fast','gpu-compare-4070ti','fast'),('4070ti_high','gpu-compare-4070ti','high_resolution'),('4090_fast','gpu-compare-4090','fast'),('4090_high','gpu-compare-4090','high_resolution')]:
 p=H/folder/'accuracy.json'
 if not p.exists():continue
 r=json.loads(p.read_text())['accuracy-sample.jpg@'+mode]['result']
 if not r:continue
 lines=r['lines']+(r.get('candidates',[]) if label.endswith('_UI') else []);oriented=r.get('coordinate_space')=='document_oriented'
 def rect(l):
  if oriented:return l['left'],l['top'],l['right'],l['bottom']
  return l['top'],r['width']-l['right'],l['bottom'],r['width']-l['left']
 fields={};detail={}
 for column,(left,right) in ref['columns'].items():
  rows=[[] for _ in range(10)]
  for l in lines:
   x0,y0,x1,y1=rect(l);x,y=(x0+x1)/2,(y0+y1)/2
   if left<x<right:
    for row in range(10):
     if ref['edges'][row]<=y<ref['edges'][row+1]:rows[row].append((y0,x0,l['text']));break
  vals=[''.join(t for _,__,t in sorted(row)) for row in rows]
  digit=column in ['birth','work','retirement']
  norm=lambda s:re.sub(r'\D' if digit else r'\s','',unicodedata.normalize('NFKC',s))
  target=ref['expected'][column]
  fields[column]=dict(correct=sum(norm(a)==norm(b) for a,b in zip(vals,target)),total=len(target))
  detail[column]=dict(observed=vals,expected=target)
 expected_ids=ref.get('expected_source_id_strings',ref.get('ids',[]))
 # Use the 13 manually checked visible synthetic strings from the prior evaluation.
 if not expected_ids:expected_ids=['110101197'+str(n).zfill(2)+'0100'+str(n).zfill(2) for n in list(range(1,11))+[12,13,14]]
 actual=set(re.findall(r'[0-9]{15,20}[Xx]?', '\n'.join(l['text'] for l in lines)))
 fields['source_numeric_strings']=dict(correct=sum(s in actual for s in expected_ids),total=len(expected_ids))
 summary[label]=dict(fields=fields,selected_cells_correct=sum(v['correct'] for k,v in fields.items() if k!='source_numeric_strings'),selected_cells_total=sum(len(v) for v in ref['expected'].values()),accepted_lines=len(r['lines']),scored_lines=len(lines),candidate_lines=len(r.get('candidates',[])),doc_angle=r.get('doc_angle'),coordinate_space=r.get('coordinate_space','original'))
 details[label]=detail
(H/'table-summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2))
(H/'table-details-private.json').write_text(json.dumps(details,ensure_ascii=False,indent=2))
print(json.dumps(summary,ensure_ascii=False,indent=2))
