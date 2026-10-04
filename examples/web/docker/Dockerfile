FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends libstdc++6 ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY app/ocr-web /app/ocr-web
COPY app/models /app/models
COPY app/runtime /app/runtime
COPY app/licenses /app/licenses
COPY config.yaml /app/config.yaml
RUN chmod 755 /app/ocr-web && chmod 600 /app/config.yaml
EXPOSE 7676
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -fsS --max-time 3 http://127.0.0.1:7676/ > /dev/null || exit 1
ENTRYPOINT ["/app/ocr-web", "-config", "/app/config.yaml"]
