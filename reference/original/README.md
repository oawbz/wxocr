# 原引擎参考

来源：https://github.com/swigger/wechat-ocr 。`UPSTREAM.md` 保留上游说明，`src/`、`pb/`、`vs.proj/` 保留原生桥接与协议代码，`libmmmojo.so` 是原有 Linux 桥接库。此目录只用于原引擎研究对照，不参与核心 Go 包或示例运行。

这不是微信 OCR 引擎的完整源码；原引擎执行仍需安装对应平台的微信及其 OCR 组件。`probes/` 是 macOS 已安装微信的 ABI 对照探针。其历史产物路径已经删除，运行时请自行提供输入和输出路径。

项目外清理备份：`/private/tmp/wxocr-before-cleanup-20261004.tar.gz`，包含原研究目录和原始参考仓库。它不是运行依赖，临时目录可能被系统清理。
