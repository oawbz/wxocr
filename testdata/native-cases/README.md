# 对照数据

`manifest.json` 记录 29 张回归图片的尺寸和 SHA256；`native_expected.json` 为 H255 Linux x86-64 微信原引擎输出的文字行，坐标及置信度以 9 位有效数字采集。它保留原引擎识别结果，不是人工正确文字标注。

`native_merge_expected.json` 为 macOS ARM64 原库 `MergeLinesToParagraphs` 在指定文字四边形与段落区域输入下的输出。该文件记录原库哈希，普通 Go 测试可重放全部 28 个非空案例，不需要本机安装微信。

六张 `stress_*.png` 可用 `tools/corpus/generate_stress.py`、Pillow 和指定中文字体重新生成；当前采集使用 macOS Arial Unicode 字体。重新生成后应重新采集原引擎基线及更新哈希。
