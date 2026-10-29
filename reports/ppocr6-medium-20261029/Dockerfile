FROM wxocr-web:cuda-fragment-20261029
USER root
RUN apt-get update && apt-get install -y --no-install-recommends python3 python3-pip libgl1 libglib2.0-0 && rm -rf /var/lib/apt/lists/*
RUN python3 -m pip install --no-cache-dir paddlepaddle-gpu==3.3.0 -i https://www.paddlepaddle.org.cn/packages/stable/cu126/
RUN python3 -m pip install --no-cache-dir paddleocr==3.7.0 fastapi uvicorn python-multipart pillow
ENV PADDLE_PDX_DISABLE_MODEL_SOURCE_CHECK=True
ENV LD_LIBRARY_PATH=/usr/local/cuda/lib64:/usr/lib/x86_64-linux-gnu
WORKDIR /test
ENTRYPOINT []
CMD ["python3"]
RUN apt-get update && apt-get install -y --no-install-recommends libgomp1 && rm -rf /var/lib/apt/lists/*
