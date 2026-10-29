#!/bin/bash
set -e
cd /home/gemini/ppocr6-test-20261029
while [ ! -f baseline-single.exit ]; do sleep 2; done
[ "$(cat baseline-single.exit)" = 0 ]
bash start_single_cpu.sh
for i in $(seq 1 120); do
 if curl -fsS --max-time 1 http://127.0.0.1:17679/health >health-single.json; then break; fi
 [ "$(docker inspect -f '{{.State.Running}}' ppocr6-medium-test)" = true ]
 sleep 2
done
python3 -c "import json; assert json.load(open('health-single.json'))['workers']==1"
python3 paddle_single.py
