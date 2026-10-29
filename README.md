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

通过 `model_resident` 选择模型加载方式，默认 `true`：启动时加载一次并常驻复用，加载失败不会开启监听。设为 `false` 时，每次识别加载模型并在结束后释放；加载错误在请求时返回，每次请求增加加载耗时，加载与推理仍串行执行。释放会话不保证进程 RSS 立即回到初始值。修改 `examples/web/config.yaml` 后重启服务：

| 配置项 | 默认值 / 说明 |
| --- | --- |
| `listen_address` | `127.0.0.1`，也支持 `0.0.0.0` / `::` |
| `listen_port` | `7676` |
| `token` | 示例 Token；部署时替换为自己的随机 Token，网页不内嵌 Token，填写后保存在当前浏览器，刷新后自动填入；清空输入框会删除保存值 |
| `max_concurrent_tasks` | `1`，在读取上传前限制整个处理流程，繁忙立即返回 429，不排队；同一引擎串行推理，建议保持 1 |
| `requests_per_minute` | `60`，全服务共享令牌桶、同数突发额度；0 关闭速率限制。仅通过鉴权且获得处理名额的 OCR 请求消耗额度 |
| `models_dir` | `../../models` |
| `model_resident` | `true`；`false` 为每次识别按需加载并释放 |
| `startup_warmup` | 默认 `false`；NVIDIA 部署为 `true`，监听前预热所有模型尺寸，要求模型常驻 |
| `runtime_library` | 空值按操作系统选择 `../../runtime` 下的动态库，Linux 需设置适配的 `.so` |
| `log_enabled` / `debug` | 均为 `false`；启动配置或模型错误仍输出到标准错误并退出 |

模型和运行库相对路径以 YAML 所在目录为基准。可使用 `go run . -config /path/to/config.yaml` 指定配置。未知字段、重复字段和无效数值在启动时拒绝。

标准接口只返回通用 OCR JSON，不判断证件类型；HTML 根据返回文字和坐标完成身份证模板匹配。页面“通过 API 调用”内附完整接口文档。

`POST /api/ocr`，请求头 `Authorization: Bearer YOUR_TOKEN`，multipart 文件字段 `image`：

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

## AMD GPU 实验部署（H255）

`examples/web/docker-amd-gpu/` 提供独立 Docker Compose、运行库适配层和验证记录。服务主体复用 Linux amd64 Go 二进制，通过固定 ORT 1.19 / ROCm 6.3.1 后端推理。H255 的 Radeon 780M 需要 gfx1100 兼容目标及关闭 SDMA；修复后已通过三组图片的文字、顺序、坐标对照和三次容器重建，但未做长期压测。当前单任务性能与 CPU 接近，GPU 镜像约 4.4 GB，不建议据此默认替换 CPU 部署。具体限制见该目录 README 和 validation.json。

## NVIDIA GPU 部署（RTX 4090）

`examples/web/docker-nvidia/` 从当前源码构建 CUDA 镜像，通过 ONNX Runtime 1.19.0、CUDA 12.6 和 cuDNN 9 加速三个模型。构建时为识别模型增加 FP32 Softmax，将归一化留在 GPU；原始模型文件保持不变。修改该目录配置中的 Token 后执行 `docker compose up -d --build`。默认宿主 GPU 1、端口 7676；通过 `OCR_GPU_ID` 和 `OCR_PORT` 调整。宿主机需要 NVIDIA 驱动和 NVIDIA Container Toolkit。实测阶段耗时、CPU 对照及回归结果见该目录 `PERFORMANCE.md`。

通过 `go run ./cmd/ocrbench -runtime /path/to/libonnxruntime.so -models models -runs 5 image.png` 测量各阶段耗时；`-cpuprofile cpu.pprof` 可采集 CPU 热点。Go 调用方可使用 `WithTimingObserver(ctx, callback)` 获取同样的分阶段计时，默认 API 响应保持原有格式。

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

### 漏识别诊断与长图召回

默认识别返回以下补充信息（正文仍使用 `lines` 和 `paragraphs`）：

- `candidates`：未达到正文阈值的非空识别结果，含原始位置、识别证据值 `evidence` 和 `reasons`。检测分数低于 0.65、检测尺度区域面积小于 300 或识别证据值不超过 0.4 时进入候选，供人工核对。触碰分块内部边缘的裁剪结果也作为 `tile_boundary` 候选保留，避免截断片段混入正文。
- `diagnostics`：每个检测区域的检测批次 `pass`、原图分块范围 `tile`、四边形 `quad`、检测分数、检测尺度面积、识别文本、证据值和处理原因。`reason` 包括 `accepted`、`candidate`、`duplicate`、`empty_recognition`、`invalid_geometry` 和 `below_candidate_floor`。检测分数低于 0.3 或区域面积小于 16 的噪声区域仅记录诊断，不进行文字识别。未形成检测框的文字不会出现在逐框诊断中。

短边达到 1600 像素的高分辨率页面，除整图检测外自动增加二维重叠局部检测：常规分块为 1280×1280，步长为边长的 80%；超大页面按最长边调整分块，最多增加 25 个局部检测批次。其他最长边超过 960 且长宽比至少为 2 的图片，自动增加重叠分块检测，支持横向和纵向长图。后一种长图的分块边长取短边与 960 的较大值，重叠取边长的 20% 且至少 160 像素。结果映射回原图坐标后按位置和文本去重；同样文字出现在不同位置时保留。额外检测和候选识别会增加长图耗时。单条超长文字的切段识别尚未包含在本次优化中。

需要核对模型实际看到的裁剪输入时，在 Go 中使用 `ocr.WithDiagnosticCrops(ctx)`，或在 Web API 请求中添加 `?diagnostic_crops=true`（可与 `preview=true` 组合）。此时 `diagnostics[].crop_png` 为实际识别输入的 PNG Base64，包含模型输入的黑色填充；默认不返回图片，以减少响应体积。诊断仅随当前结果返回，不自动保存到磁盘。

网页结果区可以展开“低分候选与诊断”查看候选文字；候选框以橙色显示，完整逐框诊断可在 JSON 中查看或下载。候选不会自动混入正文及证件字段。


### 高分辨率表格与多行文字

高分辨率页面还会在原始像素上分析检测框的文字投影：排除长表格线后，沿空白带拆开多行；对跨单元格的框，只有确认竖线在框外仍连续时才沿线拆分，避免把汉字笔画当成列边界。父框以 `split_parent` 记录，子框通过 `parent_region` 指向当前 `diagnostics` 中的父框索引；随后分别识别。

局部结果与整图结果合并时，优先保留完整局部文本，压掉同位置的重复框、短片段和已被局部结果覆盖的粗合框。为减少重复推理，一部分几何高度重叠的局部框标记为 `redundant_detection`，`duplicate_of` 指向首选诊断索引；首选框识别失败或未达到正文阈值时，仍会尝试备用框。整图结果保留作为回退。

小字采用保守例外：局部检测分数至少 0.75、识别证据至少 0.9、检测尺度区域面积至少 64 时，不再因面积不足 300 而单独降为候选。低检测分数、低识别证据、分块边缘截断仍保留候选策略。该优化会增加高分辨率页面耗时，且不保证身份证号、紧贴的日期或其他数字串完全正确。


### 数字续行与推理开销

高分辨率页面根据整图文字方向调整局部分块：沿文字行方向保留 20% 重叠，沿垂直方向保留至少 64 像素、常规为边长 1/16 的重叠，以减少同一行的重复检测。局部正文覆盖原整图框面积至少 85% 时不再重复识别该整图框；覆盖按几何并集计算，候选不计入覆盖，未覆盖的整图内容仍作回退。

对占据大部分页面的单个有框表格，从多行中提取重复列边界，只对紧随表格的无框续行沿这些列切开；列边界会在附近空白处微调，减少带入相邻字段笔画。不会按字段标签、身份证位数、校验码或预期日期填字。诊断中 `split_method: table_continuation` 和 `review_reasons` 表示列边界来自表格几何推断；`incomplete_or_ambiguous_date`、`possible_boundary_fragment` 表示格式不完整或可能带入边缘碎片，原始 OCR 文本保持原样。网页在候选区域同时显示需要核对的字段。`covered_by_local` 表示被可靠局部正文覆盖而省略推理的整图框。

普通 logits 模型使用原生 C 紧凑 CTC 证据计算：保留同样的完整字表、贪心标签、blank 概率和序列置信度所需概率，用 double 累加 softmax，不裁剪字符候选、不修改模型。随机 logits、重复标签、空白、并列值及非有限值均与 Go 完整概率参考路径测试比较，置信度允许浮点末位差异。具体模型输出文本仍需以实图回归验证。

对于检测分数较高、识别证据不足的窄小框，会有限次重试：沿文字基线补充相当于字高 15% 的左右边距，并保持页面方向，避免单字被当作竖排旋转。仅当新识别证据至少 0.8 且比原值高 0.1 才替换；`retry_method`、`retry_original_text`、`recognition_quad` 记录重试和实际采样区域。没有根据字符内容选定裁剪或补写字符。
