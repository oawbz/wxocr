# AMD GPU OCR 实验部署

此目录使用独立的 Docker Compose 项目部署 Go OCR 服务。默认端口 7677，原 CPU 服务和镜像不受影响。运行库是 AMD 官方 ONNX Runtime 1.19 / ROCm 6.3.1，可选择 ROCm 或 MIGraphX 后端执行 FP32 推理。

`ort_amdgpu.c` 是实验性的 C API 适配层，在当前 Go 绑定创建文件模型会话时显式启用所选 AMD 后端，加载失败会报错。它不改变模型或 OCR 后处理。推理算子仍可能按 ONNX Runtime 的分区结果回退 CPU，需通过 profiling 记录判断实际 GPU 执行情况。

在部署目录准备 `ocr-web`（Linux amd64 二进制）、`models/`、`config.yaml`、`cache/`。网页嵌入二进制，接口仍使用 `./api/ocr`。修改配置中的 Token，再构建与启动：

```sh
./build.sh
chmod 755 ocr-web
mkdir -p cache
docker compose -p wxocr-amd-gpu up -d
docker compose -p wxocr-amd-gpu logs --tail=100
docker compose -p wxocr-amd-gpu down
```

构建脚本中的 Python 仅解压官方 wheel 获取 C/C++ 动态库，不用于 OCR 服务。AMD 镜像及其依赖仅安装到容器，不修改宿主机驱动。

Compose 只映射 `/dev/kfd` 和一个渲染设备，不使用 privileged。GPU 服务二进制与 CPU 服务相同，以只读挂载方式覆盖 `/app/ocr-web`。更新文件后用 `docker compose -p wxocr-amd-gpu up -d --force-recreate ocr-gpu` 重建容器。

测试阶段可添加 `WXOCR_GPU_PROFILE=1` 启用 profiling；正常关闭会话时将执行记录写到 `cache/`。默认关闭 profiling，避免长期生成调试记录。显卡可被 ROCm 枚举不代表模型可执行，实验结果以实际请求、GPU 节点执行记录和 CPU/GPU 输出对比为准。

MIGraphX 模式下段落模型由 CPU 执行：MIGraphX 2.11 加载该模型时报告 `CONVOLUTION_BACKWARDS: mismatched channel numbers`。检测和识别模型继续尝试 MIGraphX；具体执行分区以验证记录为准。

Compose 当前选择 `WXOCR_GPU_PROVIDER=rocm`，并使用 `MIOPEN_FIND_MODE=FAST`。ROCm EP 属于旧版后端，本次固定使用 ORT 1.19；不能据此保证新版 ORT 或其他 AMD 显卡兼容。切换为 `migraphx` 时段落模型仍走 CPU。

H255 的实际 GPU 为 Radeon 780M / gfx1103。ROCm 6.3.1 的 rocBLAS 缺少 gfx1103 算子库，当前 Compose 使用 `HSA_OVERRIDE_GFX_VERSION=11.0.0` 选择 gfx1100 兼容目标；此配置仅验证于该设备，不应直接视为其他 AMD GPU 的通用配置。

在 1Panel 使用本 Compose 时，可设置 `OCR_BINARY_PATH`、`OCR_CONFIG_PATH`、`OCR_MODELS_PATH`、`OCR_CACHE_PATH` 为部署文件的绝对路径。默认只绑定本机；通过 `OCR_GPU_BIND_ADDRESS` 调整外部监听地址，Token 请在配置文件中设置。

H255 内核记录曾出现 `sdma_engine_id 1 exceeds maximum id of 0`、GPU Hang 和驱动自动重置。当前实验配置还设置 `HSA_ENABLE_SDMA=0`，通过计算内核完成数据复制，绕开该设备的 SDMA 路径。请以 `validation.json` 中的修复后请求和容器重建测试为准，不能将 GPU 被枚举或网页健康检查视为识别成功。
