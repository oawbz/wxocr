"""Isolated evaluation server. No access to production config or tokens."""
import os,time,json,queue,threading
os.environ.setdefault('PADDLE_PDX_DISABLE_MODEL_SOURCE_CHECK','True')
os.environ.setdefault('FLAGS_allocator_strategy','auto_growth')
os.environ.setdefault('OMP_NUM_THREADS','1')
import cv2,numpy as np
cv2.setNumThreads(1)
from fastapi import FastAPI,UploadFile,File
from paddleocr import PaddleOCR
import paddle
app=FastAPI()
engines=queue.Queue()
workers=int(os.environ.get('OCR_WORKERS','1'))
@app.on_event('startup')
def start():
 print('VERSIONS',paddle.__version__,'cuda_build',paddle.is_compiled_with_cuda(),'visible_gpus',paddle.device.cuda.device_count(),flush=True)
 assert paddle.is_compiled_with_cuda() and paddle.device.cuda.device_count()==1
 for i in range(workers):
  t=time.perf_counter()
  e=PaddleOCR(text_detection_model_name='PP-OCRv6_medium_det',text_recognition_model_name='PP-OCRv6_medium_rec',use_doc_orientation_classify=True,use_doc_unwarping=False,use_textline_orientation=True,device='gpu:0',cpu_threads=1)
  for _ in range(2): list(e.predict('/corpus/chinese.png'))
  engines.put(e)
  print('WARM',i+1,round(time.perf_counter()-t,3),flush=True)
 print('READY',workers,flush=True)
@app.get('/health')
def health():return dict(workers=workers,available=engines.qsize())
@app.post('/api/ocr')
def ocr(image:UploadFile=File(...),mode:str='default'):
 t=time.perf_counter()
 data=image.file.read()
 im=cv2.imdecode(np.frombuffer(data,np.uint8),cv2.IMREAD_COLOR)
 e=engines.get()
 try:
  params={} if mode=='default' else dict(text_det_limit_side_len=1280,text_det_limit_type='max')
  r=list(e.predict(im,**params))[0].json
  if isinstance(r,str):r=json.loads(r)
  r=r.get('res',r)
  lines=[]
  for text,conf,poly in zip(r['rec_texts'],r['rec_scores'],r['rec_polys']):
   p=np.array(poly)
   lines.append(dict(text=text,confidence=float(conf),left=float(p[:,0].min()),top=float(p[:,1].min()),right=float(p[:,0].max()),bottom=float(p[:,1].max())))
  return dict(elapsed_ms=(time.perf_counter()-t)*1000,result=dict(width=int(im.shape[1]),height=int(im.shape[0]),doc_angle=r.get('doc_preprocessor_res',{}).get('angle'),coordinate_space='document_oriented',text_det_params=r.get('text_det_params'),model_settings=r.get('model_settings'),lines=lines),raw=r)
 finally:engines.put(e)
