#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p wheel
if [[ ! -f wheel/onnxruntime/capi/libonnxruntime.so.1.19.0 ]]; then
  curl --fail --location --retry 3 -o ort.whl 'https://repo.radeon.com/rocm/manylinux/rocm-rel-6.3.1/onnxruntime_rocm-1.19.0-cp312-cp312-linux_x86_64.whl'
  python3 - <<'PY'
import zipfile
with zipfile.ZipFile('ort.whl') as archive:
    archive.extractall('wheel')
PY
fi
docker build -t wxocr-web:amd-gpu-20261004 .
