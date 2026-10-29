"""Compare selected table cells against manually checked sample values.
This is a scoped completeness check, not full-image OCR accuracy.
"""
import json
import re
import statistics
from pathlib import Path

HERE = Path(__file__).resolve().parent
BASE = HERE / 'before.json'
before_samples = json.loads(BASE.read_text())
after_samples = json.loads((HERE / 'after.json').read_text())
before = before_samples[0]['result']
after = after_samples[0]['result']

def upright(line, result):
    return (line['top'], result['width']-line['right'],
            line['bottom'], result['width']-line['left'])

# Row anchors come from the account-date cells, not from the revised detector.
anchors = []
for line in before['lines']:
    x0,y0,x1,y1 = upright(line,before)
    if line['text']=='2005-01' and 1780 < (x0+x1)/2 < 2000:
        anchors.append((y0+y1)/2)
anchors.sort()
assert len(anchors)==10
edges = [anchors[0]-(anchors[1]-anchors[0])/2]
edges += [(a+b)/2 for a,b in zip(anchors,anchors[1:])]
edges += [anchors[-1]+(anchors[-1]-anchors[-2])/2]

# Coordinates are in the upright original image (3506 x 2480), measured from
# the source grid. Only selected cells are transcribed here.
columns = {'sequence':(65,220), 'name':(225,420), 'birth':(1380,1580),
           'work':(1580,1780), 'none':(2010,2465), 'retirement':(2680,2940)}

def values(result, column):
    left,right = columns[column]
    rows = [[] for _ in anchors]
    for line in result['lines']:
        x0,y0,x1,y1 = upright(line,result)
        x,y=(x0+x1)/2,(y0+y1)/2
        if not left<x<right:
            continue
        for row in range(10):
            if edges[row] <= y < edges[row+1]:
                rows[row].append((y0,x0,line['text']))
                break
    return [''.join(text for _,__,text in sorted(row)) for row in rows]

expected = {
 'sequence':[str(i) for i in range(1,11)],
 'name':['测试人员'+str(i) for i in range(1,11)],
 'none':['无']*10,
 'birth':[str(y)+'0101' for y in list(range(1971,1980))+[1970]],
 'work':[str(y)+'0701' for y in list(range(1991,2000))+[1990]],
 'retirement':['2025'+str(m).zfill(2)+'01' for m in range(2,12)],
}
checks={}
for key,target in expected.items():
    a,b=values(before,key),values(after,key)
    normalize = (lambda s: re.sub(r'\D','',s)) if key in ('birth','work','retirement') else (lambda s: re.sub(r'\s','',s))
    checks[key]={
      'expected':target,'before':a,'after':b,
      'before_correct':sum(normalize(v)==e for v,e in zip(a,target)),
      'after_correct':sum(normalize(v)==e for v,e in zip(b,target)),
    }

summary={
 'input':'/Users/awbz/Downloads/20260713122533.jpg',
 'baseline':'table refinement snapshot before numeric and performance changes',
 'before_lines':len(before['lines']),'after_lines':len(after['lines']),
 'before_candidates':len(before.get('candidates',[])),
 'after_candidates':len(after.get('candidates',[])),
 'before_median_ms':statistics.median(s['total_ms'] for s in before_samples),
 'after_median_ms':statistics.median(s['total_ms'] for s in after_samples),
 'before_runs_ms':[s['total_ms'] for s in before_samples],
 'after_runs_ms':[s['total_ms'] for s in after_samples],
 'local_passes':max(d['pass'] for d in after['diagnostics']),
 'split_parents':sum(d['reason']=='split_parent' for d in after['diagnostics']),
 'checks':checks,
 'limitations':'Selected-cell completeness only. Date checks ignore separators. ID numbers and footer dates are not included in the score.',
}
(HERE/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
for version,result in [('before',before),('after',after)]:
    (HERE/(version+'.txt')).write_text('\n'.join(l['text'] for l in result['lines'])+'\n')
print(json.dumps({k:v for k,v in summary.items() if k!='checks'},ensure_ascii=False,indent=2))
print({key:(c['before_correct'],c['after_correct']) for key,c in checks.items()})

# These source strings are deliberately 17-digit synthetic values. Do not pad
# them to a standard ID length or apply a checksum correction.
ids=['110101197'+str(n).zfill(2)+'0100'+str(n).zfill(2) for n in list(range(1,11))+[12,13,14]]
def numeric_metrics(result):
    texts=[line['text'] for line in result['lines']]
    observed=set(re.findall(r'[0-9]{15,20}[Xx]?', '\n'.join(texts)))
    footer=[line for line in result['lines'] if (upright(line,result)[1]+upright(line,result)[3])/2>2200]
    independent_ids=[line['text'] for line in footer if re.fullmatch(r'[:：\s]*[0-9]{15,20}[Xx]?',line['text'])]
    account=[line['text'] for line in footer if 1780<(upright(line,result)[0]+upright(line,result)[2])/2<2000]
    reviews=[d for d in result.get('diagnostics',[]) if d['reason']=='accepted' and any(r!='inferred_table_column' for r in d.get('review_reasons',[]))]
    return {'source_id_digit_runs_present':sum(i in observed for i in ids),'footer_independent_id_boxes':len(independent_ids),'footer_independent_id_texts':independent_ids,'footer_account_months_correct':sum(t=='2005-01' for t in account),'review_fields':[{'text':d.get('text'),'reasons':d['review_reasons']} for d in reviews]}
summary['numeric_fields']={'expected_source_id_strings':ids,'before':numeric_metrics(before),'after':numeric_metrics(after)}
summary['diagnostic_reasons']={}
for d in after['diagnostics']:
    reason=d['reason'];summary['diagnostic_reasons'][reason]=summary['diagnostic_reasons'].get(reason,0)+1
summary['limitations']='Selected-field completeness only. Date digits ignore separators. Source IDs are 17-digit synthetic strings; obscured dates are not reconstructed.'
(HERE/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
print('numeric fields:',json.dumps(summary['numeric_fields'],ensure_ascii=False,indent=2))
