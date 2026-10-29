# NVIDIA CUDA 部署

从项目当前源码构建，使用 ONNX Runtime 1.19.0、CUDA 12.6 和 cuDNN 9。
宿主机需安装 NVIDIA 驱动、NVIDIA Container Toolkit 和 Docker Compose。

先将 `config.yaml` 的示例 Token 换成随机值，再运行：

```sh
mkdir -p cache
chmod 600 config.yaml
OCR_UID=$(id -u) OCR_GID=$(id -g) docker compose up -d --build
docker compose ps
docker compose logs --tail=100
```

默认访问 `http://服务器IP:7676`。默认选择宿主 GPU 1（新广视服务器上的 RTX 4090）；
其他机器使用 `OCR_GPU_ID=0 docker compose up -d`，也可以指定 GPU UUID。
容器只暴露选定 GPU，因此 CUDA 内部设备编号为 0。
容器默认以 UID/GID 1000 运行；配置文件和 cache 目录应属于该用户。
其他用户通过 `OCR_UID` / `OCR_GID` 指定属主对应编号。
端口通过 `OCR_PORT` 调整，监听地址通过 `OCR_BIND_ADDRESS` 调整。

适配层为每个模型显式注册 CUDA Execution Provider，注册失败即报错；
不支持的算子仍可由 ONNX Runtime 的 CPU 后端执行。
每个模型的 CUDA arena 上限为 2 GiB，不代表整个进程显存硬上限。
`model_resident: true` 默认常驻模型，`false` 每次请求加载并释放。
停止服务：`docker compose down`。

实际部署目录：`/home/gemini/wxocr-4090-20261029/examples/web/docker-nvidia`。
服务器的配置包含独立随机 Token，仅保存在服务器配置文件中。

CUDA 依赖对应关系见 [ONNX Runtime 官方文档](https://onnxruntime.ai/docs/execution-providers/CUDA-ExecutionProvider.html)。

## 2026-10-29 性能优化部署

当前默认镜像为 `wxocr-web:cuda-perf-20261029`。预处理减少重复转换和分配，识别模型在构建时追加 FP32 Softmax，让 CUDA 同时完成归一化；原始模型保持不变。
29 张回归图片的文字、顺序、坐标和段落一致。具体阶段耗时、CPU 对照、测试边界与回退方法见 [PERFORMANCE.md](PERFORMANCE.md)。
首次部署记录保留为 `validation-initial.json`；当前验证记录在 `validation.json`。
