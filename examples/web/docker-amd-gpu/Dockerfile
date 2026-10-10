FROM rocm/dev-ubuntu-24.04:6.3.1
RUN apt-get update && apt-get install -y --no-install-recommends migraphx miopen-hip rocblas hipblaslt libstdc++6 curl gcc libc6-dev && rm -rf /var/lib/apt/lists/*
RUN apt-get update && apt-get install -y --no-install-recommends hipfft && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY wheel/onnxruntime/capi/libonnxruntime*.so* /opt/ort/
COPY ort_amdgpu.c onnxruntime_c_api.h /tmp/ort-adapter/
RUN mkdir -p /app/runtime /app/cache && gcc -shared -fPIC -O2 -I/tmp/ort-adapter /tmp/ort-adapter/ort_amdgpu.c -o /app/runtime/libort-amd.so -ldl -lpthread && rm -rf /tmp/ort-adapter
ENV LD_LIBRARY_PATH=/opt/ort:/opt/rocm/lib:/opt/rocm/lib64
ENTRYPOINT ["/app/ocr-web","-config","/app/config.yaml"]
