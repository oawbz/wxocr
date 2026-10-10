# Linux arm64 / RK3566

本目录提供 CPU 版 ONNX Runtime 1.19.0 的 Docker Compose 部署材料。Home 使用原生 arm64 构建，不依赖 QEMU 或宿主机 Go。

构建上下文包含本目录的 Dockerfile、compose.yaml、.dockerignore，以及 `source/` 下的完整 Go 源码（根模块与 `examples/web` 模块）、`internal/`、`models/`、`runtime/linux-arm64/`、根目录许可文件和 `testdata/native-cases/chinese.png`。构建过程运行真实模型的 Web 测试。

```sh
docker build --platform linux/arm64 -t wxocr-web:linux-arm64-20261004-home .
docker compose -p wxocr-home up -d
docker compose -p wxocr-home ps
docker compose -p wxocr-home logs --tail=100
docker compose -p wxocr-home down
```

将 `config.yaml` 的 Token 替换成自己的随机值。`model_resident: true` 默认启动时加载并常驻；`false` 每次识别加载、结束释放。容器挂载该配置；修改后重启生效。可在 `.env` 设置 `OCR_BIND_ADDRESS` 和 `OCR_PORT`，默认 `0.0.0.0:7676`。容器内存上限 1536 MiB，默认单任务，忙时返回 429。

Home 部署目录为 `/mnt/Storage/app/wxocr`，绑定 `192.168.1.233:7676`。配置、Compose 和基准结果位于该目录；源码构建上下文位于 `build-20261004/`。

```sh
cd /mnt/Storage/app/wxocr
docker compose -p wxocr-home restart ocr
docker compose -p wxocr-home down
```

此部署使用 CPU；未转换模型或启用 RK3566 NPU。性能结果记录在本目录 `validation.json`，适用范围限于所列图片及测试状态。

Home 构建时默认 Go 模块源在容器内连接超时，使用 `docker build --network host --build-arg GOPROXY=https://goproxy.cn,direct ...` 完成构建。该选项只用于构建阶段；运行服务仍使用 Compose 的独立 bridge 网络，模块内容保持 `go.sum` 校验。

实测模型常驻、CPU 模式（各类连续三次中位数）：三行中文 9.05 秒、12 行票据 12.49 秒、18 行网页截图 12.27 秒；H255 同图 CPU 分别 0.51、0.71、0.72 秒。10 次输出文字与 H255 完全一致。服务 RSS 约 451 MiB，记录到的进程峰值约 472 MiB；重启到页面就绪约 2.17 秒（文件缓存已热）。该测试未验证长时间运行或任意大图吞吐。

`inference_threads` 控制单张图片的 CPU 模型推理线程数：默认 `1`，可指定 `2`、`4`，或 `0` 由 ONNX Runtime 自动选择。范围 `0–128`。这与 `max_concurrent_tasks` 独立，多请求仍在同一引擎中串行推理。修改配置后重新启动服务；Compose 使用 `docker compose up -d --force-recreate ocr`，避免继续使用替换前的配置挂载。多线程的提速幅度需在目标机器实测。
