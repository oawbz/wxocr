# Go OCR

核心位于模块根目录，导入路径 `wxocr`，包名 `ocr`。通过 Go API 调用模型完成图片解码、检测、识别、段落归并和阅读顺序处理；无需 Python 或微信原引擎。

## 目录

- 根目录：标准 Go OCR 包及回归测试。
- `internal/`：JPEG 解码和多边形处理所需的内部 C/C++ 源码与许可证。
- `models/`：三个 ONNX 模型、字符表和校验清单。
- `runtime/`：当前本机 macOS ARM64 ONNX Runtime 动态库。
- `examples/web/`：独立 Go 模块，Gin + 内嵌 HTML 测试页面。
- `testdata/`：核心回归测试图片和期望数据。
- `reference/original/`：一份原引擎桥接与协议参考，不参与运行。

## 调用核心

核心要求 Go 1.21+、CGO 和 C/C++ 编译器，并需要目标平台的 ONNX Runtime 动态库。当前验证平台是 macOS ARM64 和 Linux x86-64；仓库自带动态库仅适用于 macOS ARM64。示例完全以 Go 编写，核心不是 `CGO_ENABLED=0` 的纯 Go 实现。

```go
package main

import (
    "context"
    "fmt"
    "log"
    ocr "wxocr"
)

func main() {
    engine, err := ocr.New(ocr.Config{
        RuntimeLibrary: "runtime/libonnxruntime.dylib",
        DetectionModel: "models/detection.onnx",
        RecognitionModel: "models/recognition.onnx",
        ParagraphModel: "models/paragraph.onnx",
        CharsetFile: "models/charset_zh13562.txt",
    })
    if err != nil { log.Fatal(err) }
    defer engine.Close()
    result, err := engine.RecognizeFile(context.Background(), "image.jpg")
    if err != nil { log.Fatal(err) }
    for _, line := range result.Lines { fmt.Println(line.Text, line.Confidence) }
}
```

其他本地 Go 项目使用 `require wxocr v0.0.0` 并通过 `replace wxocr => /你的项目路径` 引用。发布到仓库时应将模块路径改为实际仓库地址。

同一个 Engine 的识别调用会串行执行，可复用至应用退出。`Close` 释放模型和运行时引用。支持 JPEG / PNG；JPEG 会应用 EXIF 方向，输出坐标以旋转后图片为准。置信度是长度归一化 CTC 模型分数，尚未校准为正确率。明确网格表格支持按行列排序，复杂无边框排版仍需进一步验证。

## Gin 网页示例

需要 Go 1.25+。从项目根目录运行：

```sh
cd examples/web
go run .
```

访问 http://127.0.0.1:7676，在网页填写 `config.yaml` 中的 Token 后上传图片。无需 Node、Python 或前端构建。页面适配桌面和手机，支持拖放、粘贴截图、识别框开关、文字复制、顺序与置信度、JSON 下载。

HTML、CSS 和 JavaScript 使用 `go:embed` 编译进可执行文件，无需部署 `index.html` 或其他静态文件。YAML 配置、模型和 ONNX Runtime 动态库仍为外部文件。

启动时加载一次模型并常驻复用；加载失败则不会开启监听。修改 `examples/web/config.yaml` 后重启服务：

| 配置项 | 默认值 / 说明 |
| --- | --- |
| `listen_address` | `127.0.0.1`，也支持 `0.0.0.0` / `::` |
| `listen_port` | `7676` |
| `token` | 示例 Token；部署时替换为自己的随机 Token，网页不内嵌 Token，填写后保存在当前浏览器，刷新后自动填入；清空输入框会删除保存值 |
| `max_concurrent_tasks` | `1`，在读取上传前限制整个处理流程，繁忙立即返回 429，不排队；同一引擎串行推理，建议保持 1 |
| `requests_per_minute` | `60`，全服务共享令牌桶、同数突发额度；0 关闭速率限制。仅通过鉴权且获得处理名额的 OCR 请求消耗额度 |
| `models_dir` | `../../models` |
| `runtime_library` | 空值按操作系统选择 `../../runtime` 下的动态库，Linux 需设置适配的 `.so` |
| `log_enabled` / `debug` | 均为 `false`；启动配置或模型错误仍输出到标准错误并退出 |

模型和运行库相对路径以 YAML 所在目录为基准。可使用 `go run . -config /path/to/config.yaml` 指定配置。未知字段、重复字段和无效数值在启动时拒绝。

标准接口：`POST /api/ocr`，请求头 `Authorization: Bearer YOUR_TOKEN`，multipart 文件字段 `image`：

```sh
curl -X POST http://127.0.0.1:7676/api/ocr \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "image=@example.png"
```

成功响应为 `{ "result": { "width": ..., "height": ..., "lines": [...], "paragraphs": [...] }, "elapsed_ms": ... }`。默认不生成预览；`?preview=true` 额外返回 `preview_png`，为引擎解码、校正 EXIF 方向后 PNG 的 Base64。网页使用同一 API，请求预览用于识别框对齐。

失败响应统一为 `{ "error": { "code": "...", "message": "..." } }`：401 鉴权失败，400 图片或参数无效，413 完整上传超过 20 MiB，429 并发或速率超限，503 服务停止中，504 识别超时。429 携带 `Retry-After` 秒数。图片最多 2500 万像素，仅支持 JPEG / PNG。

服务设置请求头、上传、响应与空闲连接超时，及时关闭上传文件并清理 multipart 临时文件。识别上下文限时 90 秒；取消在推理阶段之间检查，正在执行的原生推理不能强制中断。停止时先拒绝新任务，等待已有任务结束后释放模型，避免仍在使用时销毁运行库。HTTP 接口默认仅本机访问，跨设备部署可配置监听地址；公网使用时应通过 HTTPS 反向代理传输 Token。

## 交互式编译打包

在 `examples/web` 目录运行 `./build.sh`，依次选择 macOS / Linux 和 arm64 / amd64，回车默认当前系统、当前架构。输出到 `examples/web/dist/平台-架构-时间/`，包含可执行文件、YAML 配置、模型、运行库和 `start.sh`；HTML/CSS/JS 已嵌入程序。每次创建新目录，不覆盖之前的编译结果。`start.sh` 默认后台运行，再次执行会先安全停止旧实例再启动；PID 写入 `ocr-web.pid`，已启用的日志和启动错误写入 `ocr-web.log`。

跨平台编译需要目标 C/C++ 工具链（可通过 `CC`、`CXX` 指定）及目标 ONNX Runtime。将动态库放到 `runtime/linux-amd64/libonnxruntime.so` 等对应目录，或通过 在 `examples/web` 下通过 `ORT_LIBRARY=/绝对路径/动态库 ./build.sh` 指定；脚本会检查格式和架构。目标系统还需兼容的 libc 和 C++ 运行库；仅 macOS ARM64、Linux amd64 已验证运行。

## Docker 部署（Linux amd64）

Docker 部署材料在 `examples/web/docker/`，已打包的镜像与 Compose 在 `examples/web/dist/docker-linux-amd64-idcard-20261004/`。镜像自带程序、模型、ONNX Runtime 和 Debian 12 基础库，运行时无需宿主机 Go 或 ONNX Runtime。

在部署目录修改 `config.yaml` 的 Token 后运行：

```sh
docker load -i wxocr-web-linux-amd64.tar.gz
docker compose up -d
```

默认访问 `http://服务器IP:7676`。内部监听固定 `0.0.0.0:7676`，主机端口可通过 `OCR_PORT=8080 docker compose up -d` 修改。配置修改后 `docker compose restart ocr`，停止服务用 `docker compose down`。具体配置和验证边界见部署 README 与 `validation.json`；容器部署已经在 H255 实测，未在用户的 Debian 主机实测。

## 测试

```sh
go test -race ./...
go vet ./...
cd examples/web
go test -race ./...
go vet ./...
```

真实模型回归另需设置 `WXOCR_RUNTIME`、`WXOCR_DETECTOR`、`WXOCR_RECOGNIZER`、`WXOCR_PARAGRAPH`、`WXOCR_CHARSET` 和 `WXOCR_NATIVE_CORPUS=1`。历史 108 张图片在两平台输出文字与顺序一致，这不是全部识别正确的证明。仍存在小字误识别，置信度需要更多人工真值校准。

模型与第三方代码的权利归各自权利人；内部组件及 OpenCV / Clipper 的许可证保留在原目录及根目录。
