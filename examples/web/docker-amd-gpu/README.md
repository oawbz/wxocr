# AMD GPU OCR 实验部署

此目录使用独立的 Docker Compose 项目部署 Go OCR 服务。默认端口 7677，原 CPU 服务和镜像不受影响。运行库是 AMD 官方 ONNX Runtime 1.19 / ROCm 6.3.1，通过 MIGraphX 执行 FP32 推理。

`ort_migraphx.c` 是实验性的 C API 适配层，在当前 Go 绑定创建会话选项时显式启用 MIGraphX，加载失败会报错。它不改变模型或 OCR 后处理。推理算子仍可能按 ONNX Runtime 的分区结果回退 CPU，需通过 profiling 记录判断实际 GPU 执行情况。

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

测试阶段 `WXOCR_GPU_PROFILE=1` 启用 profiling；正常关闭会话时将执行记录写到 `cache/`。验证后可移除该环境变量并重建容器，避免持续生成调试记录。显卡可被 ROCm 枚举不代表模型可执行，实验结果以实际请求、GPU 节点执行记录和 CPU/GPU 输出对比为准。
