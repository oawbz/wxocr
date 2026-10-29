# RTX 4090 阶段耗时分析与优化（2026-10-29）

已部署 `wxocr-web:cuda-perf-20261029`，访问 `http://10.100.5.250:7676/`。
宿主 GPU 1 为 RTX 4090，Compose 容器以 UID/GID 1000 运行，健康且无重启。

## 测量方法

同机、同图、同一个 FP32 推理运行库；各模式顺序执行，逐图先预热一次、再测 5 次取中位数。
优化 CPU 与优化 GPU 使用同一个新增 Softmax 的模型图，CPU 不注册 CUDA Provider。
关闭 CPU 和 ONNX 算子性能采集后复测；计时包含解码及全部 OCR 阶段，不含 HTTP 上传、JSON 编码和网页预览 PNG 编码。
服务器其他业务保持运行，结果不是独占 GPU 的压力测试。阶段中位数之和不必等于总耗时中位数。

## 总耗时（毫秒）

| 图片 | 优化前 GPU | 优化后 GPU | 相对原 GPU 提速 | 优化后 CPU | GPU 相对优化 CPU 提速 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 中文 | 636.3 | 243.5 | 2.61× | 517.5 | 2.13× |
| 密集票据 | 1115.1 | 459.3 | 2.43× | 772.6 | 1.68× |
| 双栏 | 986.6 | 407.5 | 2.42× | 719.9 | 1.77× |

## 各阶段（密集票据，毫秒）

| 阶段 | 优化前 GPU 路径 | 优化后 GPU 路径 |
| --- | ---: | ---: |
| 图片解码 | 7.82 | 6.82 |
| 检测预处理 | 200.88 | 42.60 |
| 检测推理与输出复制 | 11.89 | 14.24 |
| 检测框后处理 | 16.54 | 12.60 |
| 文字行透视裁剪 | 43.46 | 18.41 |
| 文字识别推理与输出复制 | 142.76 | 147.91 |
| 归一化 / CTC 解码 | 438.94 | 91.11 |
| 段落预处理 | 151.61 | 31.22 |
| 段落推理与输出复制 | 10.72 | 10.89 |
| 段落区域后处理 | 62.41 | 68.00 |
| 段落和表格阅读顺序 | 6.91 | 10.27 |

## 瓶颈与实现

1. 原预处理反复调用 `image.Image.At`、颜色转换与逐像素插值参数计算，还产生大量小对象。改为 NRGBA / RGBA / Gray 像素缓冲直接读取，保留透明像素原始 RGB；预计算两轴固定点插值参数、复用水平缩放行，并用 256 项查表完成完全相同的归一化。透视裁剪也复用精确像素读取路径。
2. 原识别模型只输出 logits，CPU 用 Go 对每行的 13,564 类执行 Float64 Exp/Softmax。CUDA 镜像构建时仅为识别模型增加末维 FP32 Softmax，输出显式命名为 `probabilities`，核心根据模型签名跳过重复的 CPU Softmax；CTC、接受阈值与置信度算法保持原有逻辑。原始 ONNX 文件不修改，CPU 原模型仍受支持。算子采集确认新增 Softmax 使用 `CUDAExecutionProvider`。
3. 新增可选 `WithTimingObserver` 与 `cmd/ocrbench`，默认 HTTP 响应不增加性能字段。模型推理阶段含输入 Tensor 创建、原生调用及结果复制，进一步的算子拆分由 ORT profile 提供。

预处理是两种后端共有的优化；GPU 的优势通过与同样优化后的 CPU 比较体现。剩余主要开销包括逐行推理/输出复制、CTC 与段落后处理，后续不能仅靠提高显卡利用率判断收益。

## 正确性与实际服务

- 29 张图片：文字、顺序、坐标及段落结构全部一致；置信度最大绝对差异约 1.5e-6。
- 预处理与旧实现逐值相等测试通过，覆盖透明像素、不同颜色格式、偏移坐标和子图 stride；透视裁剪快速路径与通用路径逐值相等。
- 根模块和 Web 模块的 `go test -race ./...`、`go vet ./...` 通过。
- HTTP 页面 200、未鉴权请求 401；部署后第二次请求中文 / 票据 / 双栏分别 262 / 465 / 423 ms。
- 部署后首个中文请求为 912 ms，存在 CUDA 首次执行开销，未计入热请求中位数。
- 服务未开启性能采集。原镜像保留为 `wxocr-web:cuda-before-perf-20261029`。

## 复现

```sh
go build -o /tmp/ocrbench ./cmd/ocrbench
/tmp/ocrbench -runtime /path/to/libonnxruntime.so -models /path/to/models -runs 5 image.png
# CPU 热点：增加 -cpuprofile /tmp/cpu.pprof
# go tool pprof -top /tmp/ocrbench /tmp/cpu.pprof
```

CUDA 配置目录内重新构建并启动：`docker compose up -d --build`。
回退到服务器上保留的原镜像：`OCR_IMAGE=wxocr-web:cuda-before-perf-20261029 docker compose up -d --no-build`。

完整阶段数据见仓库 `reports/perf-20261029/summary.json`，服务器同路径还保留原始样本、CPU profile、算子记录、构建与验证日志。
[ONNX Softmax 语义](https://onnx.ai/onnx/operators/onnx__Softmax.html)。
