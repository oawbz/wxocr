# OCR 测试示例

纯 Go 服务，页面嵌入二进制，默认启动时加载并复用 OCR 模型。配置 `model_resident: false` 可改为每次识别加载、结束后释放；修改后重启服务生效，每次识别会增加模型加载耗时。

接口使用 `POST /api/ocr`，Bearer Token 鉴权，multipart 文件字段 `image`；可选查询参数 `preview=true`。成功和失败均返回标准 JSON，后端只做 OCR，不判断文档类型。完整接口字段、响应示例与错误码见页面“通过 API 调用”下的接口文档。

HTML 根据 OCR 文字和坐标匹配身份证正反面并提取字段，“识别类型”标签页在“纯文字”之前。普通图片仍显示全部文字。类型判断属于启发式模板，缺失字段提示核对；不判断证件真伪。下载 JSON 为原始 OCR 响应，不含网页模板字段。

默认端口 7676、同时处理一个任务、关闭日志及 debug；Token、监听及限流在 config.yaml 配置。Docker Compose 见 docker/。

### 漏识别核对

识别结果额外包含 `result.candidates`（低分候选）和 `result.diagnostics`（逐检测框诊断）。网页可展开候选列表，橙色框对应待核对内容，JSON 保留过滤及合并原因。长图自动使用整图加重叠分块检测，并合并重复结果。

API 可传 `diagnostic_crops=true` 返回每个已识别框的实际模型输入 PNG Base64，例如 `/api/ocr?preview=true&diagnostic_crops=true`。裁剪图默认关闭，开启后响应会变大。参数仅接受 `true` / `false`；非法值返回 HTTP 400 和 `invalid_diagnostic_crops`。

短边至少 1600 像素的页面会自动使用二维重叠局部检测，并在原像素上拆分多行合框、跨表格线合框。高分辨率页面耗时会增加，数字内容仍需核对。诊断中的 `split_parent`、`parent_region`、`redundant_detection` 和 `duplicate_of` 可用于追踪拆分和推理去重。

无框续行会根据主表重复列边界分开识别；JSON 中的 `review_reasons` 和网页“待核对字段”提示不完整日期或可能混入的边缘笔画。不会自动补全隐藏数字。高分辨率页面采用文字方向相关的重叠分块、几何覆盖回退和紧凑 logits 解码，减少重复工作。

### 识别模式

默认使用快速（通用）模式，仅整图识别。高分辨率模式保留额外分块、区域拆分和窄框重试，适合密集小字与表格，但耗时更长。网页默认选择快速，可在上传区切换。

HTTP：`POST /api/ocr?mode=fast`（可省略）或 `POST /api/ocr?mode=high_resolution`，仍以 multipart 字段 `image` 上传。成功响应新增顶层 `mode`；无效模式返回 HTTP 400 / `invalid_mode`。模式按请求生效，支持并发混用，不重载模型。

Go：`engine.Recognize(ocr.WithHighResolution(ctx, true), img)` 开启高分辨率；不设置时默认快速。

### 文字框与分数展示

网页把正文与其他已识别文字框统一为同一种框线，框旁显示编号和模型分数；“文字框与分数”列表使用同一编号，不再单列黄色低分候选。文字框数统计全部可显示框，纯文字仍输出原有正文。分数是模型证据，不是正确率。日期残缺等“待核对字段”提示保留；API 数据结构及正文筛选规则不变。
