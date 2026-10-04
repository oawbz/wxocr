# ONNX Runtime 动态库

仓库包含 ONNX Runtime **1.19.0 CPU 版**，来自 [Microsoft 官方发布](https://github.com/microsoft/onnxruntime/releases/tag/v1.19.0)。下载来源、压缩包和主库 SHA256 记录在 `manifest.json`；各平台目录附带 LICENSE 和第三方声明。

| 系统 / 架构 | 动态库 |
| --- | --- |
| macOS arm64 | `darwin-arm64/libonnxruntime.dylib` |
| macOS amd64 | `darwin-amd64/libonnxruntime.dylib` |
| Linux amd64 | `linux-amd64/libonnxruntime.so` |
| Linux arm64 | `linux-arm64/libonnxruntime.so` |

macOS arm64 路径链接到原有的 `runtime/libonnxruntime.dylib`，兼容已有配置且避免重复存储。

`examples/web/build.sh` 会根据所选平台、架构自动使用对应目录，无需另行下载运行库。跨平台编译仍需要相应 C/C++ 工具链。直接运行服务时，在 YAML 的 `runtime_library` 中填写对应库路径（相对路径以 YAML 文件位置为基准）。例如 `examples/web/config.yaml` 在 Linux amd64 使用：

```yaml
runtime_library: ../../runtime/linux-amd64/libonnxruntime.so
```

Go API 的 `RuntimeLibrary` 使用所选动态库路径。Linux 库依赖兼容的 glibc 和 C++ 运行库；macOS 官方 1.19.0 包要求 macOS 12 或更高。这些 CPU 库不包含 CUDA、ROCm 或 MIGraphX 依赖。

实际 OCR 已验证的平台为 macOS arm64、Linux amd64。macOS amd64、Linux arm64 仅核对官方来源、二进制格式与架构，尚未在对应机器验证推理。
