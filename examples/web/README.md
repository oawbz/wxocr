# OCR 测试示例

纯 Go 服务，页面嵌入二进制，默认启动时加载并复用 OCR 模型。配置 `model_resident: false` 可改为每次识别加载、结束后释放；修改后重启服务生效，每次识别会增加模型加载耗时。

接口使用 `POST /api/ocr`，Bearer Token 鉴权，multipart 文件字段 `image`；可选查询参数 `preview=true`。成功和失败均返回标准 JSON，后端只做 OCR，不判断文档类型。完整接口字段、响应示例与错误码见页面“通过 API 调用”下的接口文档。

HTML 根据 OCR 文字和坐标匹配身份证正反面并提取字段，“识别类型”标签页在“纯文字”之前。普通图片仍显示全部文字。类型判断属于启发式模板，缺失字段提示核对；不判断证件真伪。下载 JSON 为原始 OCR 响应，不含网页模板字段。

默认端口 7676、同时处理一个任务、关闭日志及 debug；Token、监听及限流在 config.yaml 配置。Docker Compose 见 docker/。

`inference_threads` 控制单张图片的 CPU 模型推理线程数：默认 `1`，可指定 `2`、`4`，或 `0` 由 ONNX Runtime 自动选择。范围 `0–128`。这与 `max_concurrent_tasks` 独立，多请求仍在同一引擎中串行推理。修改配置后重新启动服务；Compose 使用 `docker compose up -d --force-recreate ocr`，避免继续使用替换前的配置挂载。多线程的提速幅度需在目标机器实测。
