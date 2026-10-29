#!/bin/bash
set -e
cd /home/gemini/ppocr6-test-20261029
while [ ! -f paddle-single.exit ]; do sleep 2; done
[ "$(cat paddle-single.exit)" = 0 ]
docker logs ppocr6-medium-test >paddle-single-default-startup.log 2>&1
docker inspect -f '{{json .State}}' ppocr6-medium-test >paddle-single-default-state.json
docker stop -t 30 ppocr6-medium-test
docker rename ppocr6-medium-test ppocr6-medium-single-native-finished
bash start_single_cpu.sh
for i in $(seq 1 120); do
 if curl -fsS --max-time 1 http://127.0.0.1:17679/health >health-single1280.json; then break; fi
 [ "$(docker inspect -f '{{.State.Running}}' ppocr6-medium-test)" = true ]
 sleep 2
done
python3 -c "import json; assert json.load(open('health-single1280.json'))['workers']==1"
OCR_TEST_MODE=limit1280 python3 paddle_single.py
