FROM python:3.12-slim AS model-build
RUN pip install --no-cache-dir onnx==1.17.0
COPY models/recognition.onnx /models/recognition.onnx
COPY examples/web/docker-nvidia/prepare_models.py /prepare_models.py
RUN python /prepare_models.py /models/recognition.onnx /optimized/recognition.onnx

FROM golang:1.25-bookworm AS ort-download
RUN curl -fsSL --retry 3 https://github.com/microsoft/onnxruntime/releases/download/v1.19.0/onnxruntime-linux-x64-gpu-1.19.0.tgz -o /tmp/ort.tgz && mkdir -p /out/ort && tar -xzf /tmp/ort.tgz -C /out/ort --strip-components=1

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum *.go ./
COPY internal/ ./internal/
COPY examples/web/*.go examples/web/go.mod examples/web/go.sum examples/web/index.html ./examples/web/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build cd examples/web && CGO_ENABLED=1 go build -trimpath -o /out/ocr-web .
COPY examples/web/docker-nvidia/ort_cuda.c examples/web/docker-amd-gpu/onnxruntime_c_api.h /tmp/adapter/
RUN gcc -shared -fPIC -O2 -I/tmp/adapter /tmp/adapter/ort_cuda.c -o /out/libort-cuda.so -ldl -lpthread

FROM nvidia/cuda:12.6.3-cudnn-runtime-ubuntu22.04
RUN apt-get update && apt-get install -y --no-install-recommends libstdc++6 ca-certificates curl && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/ocr-web /app/ocr-web
COPY --from=build /out/libort-cuda.so /app/runtime/libort-cuda.so
COPY --from=ort-download /out/ort/lib/ /opt/ort/
COPY models/ /app/models/
COPY --from=model-build /optimized/recognition.onnx /app/models/recognition.onnx
RUN chmod -R a+rX /app/models
ENV LD_LIBRARY_PATH=/opt/ort:/usr/local/cuda/lib64
HEALTHCHECK --interval=30s --timeout=5s --start-period=90s --retries=3 CMD curl -fsS http://127.0.0.1:7676/ > /dev/null || exit 1
ENTRYPOINT ["/app/ocr-web", "-config", "/app/config.yaml"]
