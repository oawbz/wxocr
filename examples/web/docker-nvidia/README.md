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

## 当前识别率优化与双实例部署

默认镜像为 `wxocr-web:cuda-fragment-20261029`，识别候选筛选和概率收集在 CUDA 上执行，减少结果回传及 CPU CTC 扫描。`startup_warmup: true` 在监听前预热全部模型尺寸，要求 `model_resident: true`；按需加载模式需关闭启动预热。

29 张图片的文字、顺序、坐标、段落及置信度与上一版完全一致。热请求对照、首请求对照、阶段耗时和回退方法见 [COMPACT_CTC.md](COMPACT_CTC.md)。
当前记录为 `validation.json`，上一轮紧凑 CTC 记录为 `validation-compact.json`，更早记录为 `validation-perf.json`，首次部署记录为 `validation-initial.json`。

预处理使用保持相同舍入的原生定点缩放，段落后处理使用可分离形态学及扫描线统计。`engine_workers: 2` 与 `max_concurrent_tasks: 2` 配合两个独立常驻引擎；各引擎均在监听前预热。全服务 `requests_per_minute: 60` 保留，超出并发上限时立即返回 429，不排队。

最新耗时、双路压测、应用显存/CPU/内存和回退说明见 [NATIVE_PREPOST.md](NATIVE_PREPOST.md)。

最新识别率改动已上线，包括高分辨率分块、表格续行拆分、候选与待核对提示。原有 29 张语料正文与旧 GPU 版一致，表格样图前 10 行六组字段完整度均为 10/10；该大图部署后约 2.45 秒，原版约 0.85 秒。完整验证、已知参考差异和回退见 [上线记录](../../../reports/accuracy-deploy-20261029/RESULT.md)。`validation.json` 为本次记录，上一版性能记录在 `validation-native.json`。

## 快速 / 高分辨率开关

当前镜像 `wxocr-web:cuda-modes-20261029`。网页默认快速（通用）；接口默认 `mode=fast`，显式 `mode=high_resolution` 开启上一版增强识别。响应新增 `mode`，非法值返回 400 / `invalid_mode`。支持同一双实例服务并发混用模式，保留原 Token、预热及限速。实测和回退见 [模式上线记录](../../../reports/modes-20261029/RESULT.md)。`validation.json` 为当前记录，上一识别率版本为 `validation-accuracy.json`。

## 当前并发与资源上限

线上已配置 8 个独立常驻引擎、最多 8 个并发任务，`requests_per_minute: 0` 关闭每分钟限速；默认快速模式不变。CPU 没有容器配额或绑核，GPU 没有算力百分比配额。`mem_limit: 4g` 指系统内存 RAM，不含 GPU 显存；8 路压测内存最大采样约 1.80 GiB，暂保留此上限。CUDA 的 `gpu_mem_limit` 是每模型会话 2 GiB 的内存池设置，不是整个应用的显存硬上限；8 路应用显存实测约 4.68 GiB。完整压测见 [8 路记录](../../../reports/concurrency8-20261029/RESULT.md)。

## 当前通用片段处理与分数展示

镜像为 `wxocr-web:cuda-fragment-20261029`。按文本包含、相对几何和分隔线做通用去重及原像素重识别，不含图像专用坐标、词表或字段值修正。网页统一框线和分数展示；原始观察保留在诊断中。默认快速、8 路不限速配置保持。测试、限制及回退见 [通用片段记录](../../../reports/fragment-20261029/RESULT.md)。
