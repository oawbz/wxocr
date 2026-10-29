# 8 路上线与取消速率限制（2026-10-29）

线上配置已改为 `engine_workers: 8`、`max_concurrent_tasks: 8`、`requests_per_minute: 0`；默认快速模式、模型常驻、启动预热、原 Token 保留。镜像仍为 `wxocr-web:cuda-modes-20261029`。

系统内存上限继续为 4 GiB（RAM），不是显存限制。CPU 没有容器核心配额或绑核，GPU 没有算力百分比配额。ONNX Runtime 每模型会话的 CUDA 内存池设置为 2 GiB，不是进程总显存硬上限，也不涵盖所有 CUDA 分配；显存实际受整张 GPU 及其他进程占用约束。

上线验证：8 路同时混合快速和高分辨率识别，完整结果 SHA256 与对应单请求一致；随后 72 次请求在约 1.12 秒内全部成功，验证旧每分钟 60 次限速已关闭。服务 healthy、重启 0、未 OOM。验证后应用显存 4820 MiB，容器内存采样 1.144 GiB；之前 8 路持续压测内存最大采样约 1.80 GiB，4 GiB 尚有余量。

当前仍有 8 个同时任务的上限，超出会返回 429；取消的是每分钟速率限制，并未设置无限推理实例。GPU 繁忙时的实际利用率由工作负载决定。

[验证数据](verification.json)、[验证脚本](verify.py)。服务器备份和日志位于原部署目录 `reports/concurrency8-live-20261029/`，`config-before.yaml` 保存原 2 路 / 60 次每分钟配置（含私有 Token，仅留服务器）。恢复该配置并在 Compose 目录执行 `docker compose up -d --force-recreate --no-build` 可回退。
