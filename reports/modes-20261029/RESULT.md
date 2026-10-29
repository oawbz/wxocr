# 快速 / 高分辨率模式上线（2026-10-29）

镜像 `wxocr-web:cuda-modes-20261029`，服务 http://10.100.5.250:7676/，容器 healthy、重启次数 0。

网页新增识别模式选择，默认“快速（通用）”。快速模式仅整图检测；高分辨率模式恢复上一版额外分块、区域拆分和窄框重试。模式只作用于单次请求，不修改共享引擎，保留双实例、模型常驻和启动预热。每分钟 60 次限速保留。

## API

- `POST /api/ocr` 或 `POST /api/ocr?mode=fast`：快速模式，默认。
- `POST /api/ocr?mode=high_resolution`：高分辨率模式。
- 请求头和 multipart 字段 `image` 保持原样，可与 `preview`、`diagnostic_crops` 同时使用。
- 成功 JSON 增加顶层 `mode`，无效模式在读取上传内容及调用模型前返回 400 / `invalid_mode`。
- Go SDK 不设置选项时默认快速；用 `ocr.WithHighResolution(ctx, true)` 显式开启增强，false 可覆盖父上下文的选项。

## 验证

核心和 Web 的竞争检测、静态检查、网页脚本测试通过；测试覆盖默认、两种显式选项、非法输入、上下文隔离及取消。服务器候选两种模式各跑 29 张样本，正文文字、顺序、坐标、段落及置信度与原 GPU 基线完全一致。上线后快速模式再次跑 29 张，全部一致。网页 HTML 与冻结源码完全一致，默认选项为 fast。

高分辨率表格原图：默认与显式快速的完整结果一致，148 个正文框；高分辨率输出 214 框，完整结果与上一版识别率版本逐值相同。上线单次对照为快速 1022 ms、高分辨率 2365 ms（不是多次均值或容量承诺）。两个实例混合处理 fast / high_resolution 时，各请求结果与其单模式结果一致，没有串模式或重载模型。

完整原始记录见 [summary.json](summary.json)，可复验脚本为 [verify.py](verify.py)，冻结源码哈希为 [source-sha256.json](source-sha256.json)。构建及验证目录为服务器 `/home/gemini/wxocr-modes-20261029/reports/modes-20261029/`。已验证源码同步到原 Compose 构建目录，配置与 Token 保留。

旧镜像 `wxocr-web:cuda-accuracy-20261029` 保留；恢复原部署目录 `reports/modes-20261029/compose-before.yaml` 并执行 `docker compose up -d --no-build` 可回退运行版本。旧源码和配置备份也在该服务器目录，配置含 Token，不复制到仓库。
