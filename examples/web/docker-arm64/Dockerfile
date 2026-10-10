FROM golang:1.25-bookworm AS build
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY source/ /src/
WORKDIR /src/examples/web
RUN GOMAXPROCS=2 CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -p 2 -trimpath -o /out/ocr-web .
RUN GOMAXPROCS=2 WXOCR_RUNTIME=/src/runtime/linux-arm64/libonnxruntime.so go test -trimpath -p 2 ./...

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends libstdc++6 ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/ocr-web /app/ocr-web
COPY source/models/ /app/models/
COPY source/runtime/linux-arm64/ /app/runtime/
COPY source/LICENSE.opencv source/LICENSE.clipper /app/licenses/
COPY source/internal/jpegcore/LICENSE.md /app/licenses/libjpeg-turbo.md
COPY source/internal/jpegcore/README.ijg /app/licenses/README.ijg
COPY source/internal/polygonclip/LICENSE /app/licenses/polygonclip.txt
COPY --from=build /go/pkg/mod/github.com/yalue/onnxruntime_go@v1.12.1/LICENSE /app/licenses/onnxruntime-go.txt
EXPOSE 7676
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD curl -fsS --max-time 3 http://127.0.0.1:7676/ > /dev/null || exit 1
ENTRYPOINT ["/app/ocr-web", "-config", "/app/config.yaml"]
