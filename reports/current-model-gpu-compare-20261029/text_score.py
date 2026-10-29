"""Score manually transcribed text, without treating either OCR as truth.
Order-independent minimum-cost line matching; fragmentation is penalized.
Whitespace is omitted and Unicode NFKC is applied identically to both sides.
This metric does not score reading order or geometric detection IoU.
"""
import json,re,unicodedata
from pathlib import Path
H=Path(__file__).resolve().parent

def norm(s):return re.sub(r'\s+','',unicodedata.normalize('NFKC',s))
def edit(a,b):
 d=list(range(len(b)+1))
 for i,x in enumerate(a,1):
  e=[i]
  for j,y in enumerate(b,1):e.append(min(d[j]+1,e[j-1]+1,d[j-1]+(x!=y)))
  d=e
 return d[-1]
def assignment(a):
 # Hungarian assignment for a square nonnegative integer cost matrix.
 n=len(a);u=[0]*(n+1);v=[0]*(n+1);p=[0]*(n+1);way=[0]*(n+1)
 for i in range(1,n+1):
  p[0]=i;j0=0;minimum=[float('inf')]*(n+1);used=[False]*(n+1)
  while True:
   used[j0]=True;i0=p[j0];delta=float('inf');j1=0
   for j in range(1,n+1):
    if not used[j]:
     cur=a[i0-1][j-1]-u[i0]-v[j]
     if cur<minimum[j]:minimum[j]=cur;way[j]=j0
     if minimum[j]<delta:delta=minimum[j];j1=j
   for j in range(n+1):
    if used[j]:u[p[j]]+=delta;v[j]-=delta
    else:minimum[j]-=delta
   j0=j1
   if p[j0]==0:break
  while True:
   j1=way[j0];p[j0]=p[j1];j0=j1
   if j0==0:break
 return [(p[j]-1,j-1) for j in range(1,n+1)]
def score(truth,observed):
 t=[norm(s) for s in truth];o=[norm(s) for s in observed];n=len(t);m=len(o);N=n+m
 cost=[[edit(t[i],o[j]) if i<n and j<m else len(t[i]) if i<n else len(o[j]) if j<m else 0 for j in range(N)] for i in range(N)]
 pairs=assignment(cost) if N else []
 errors=sum(cost[i][j] for i,j in pairs);matches=[];exact=0
 for i,j in pairs:
  a=truth[i] if i<n else '';b=observed[j] if j<m else ''
  if i<n and j<m and t[i]==o[j]:exact+=1
  if cost[i][j]:matches.append(dict(expected=a,observed=b,edit_distance=cost[i][j]))
 chars=sum(map(len,t))
 return dict(reference_characters=chars,character_errors=errors,line_matching_cer=errors/chars if chars else None,exact_lines=exact,reference_lines=n,observed_lines=m,differences=matches)
