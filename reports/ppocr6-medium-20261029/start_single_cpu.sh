#!/bin/bash
set -e
cd /home/gemini/ppocr6-test-20261029
while [ ! -f build.exit ]; do sleep 2; done
[ "$(cat build.exit)" = 0 ]
mkdir -p cache/paddlex cache/general
docker run -d --name ppocr6-medium-test --user 1000:1000 -e HOME=/tmp --no-healthcheck --gpus device=1 --memory 4g --cpus 1 --pids-limit 512 --read-only --tmpfs /tmp:size=512m,mode=1777 --cap-drop ALL --security-opt no-new-privileges:true -e OCR_WORKERS=1 -e OMP_NUM_THREADS=1 -e OPENBLAS_NUM_THREADS=1 -e MKL_NUM_THREADS=1 -e NUMEXPR_NUM_THREADS=1 -e FLAGS_gpu_memory_limit_mb=6000 -e FLAGS_allocator_strategy=auto_growth -e PADDLE_PDX_DISABLE_MODEL_SOURCE_CHECK=True -p 127.0.0.1:17679:8000 -v "$PWD:/test:ro" -v "$PWD/cache/paddlex:/tmp/.paddlex" -v "$PWD/cache/general:/tmp/.cache" -v /home/gemini/wxocr-4090-20261029/testdata/native-cases:/corpus:ro ppocr6-medium:test-20261029 python3 -m uvicorn server:app --host 0.0.0.0 --port 8000
