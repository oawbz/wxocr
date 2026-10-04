# 原引擎段落归并对照

`paragraph_merge.cpp` 是开发验证工具，不参与 Go 运行时。它直接调用已安装的 macOS ARM64 微信库的 `MergeLinesToParagraphs`，核对相同输入下的分组与顺序。

当前观察版本的 OCRText 为 80 字节，文字字符串位于偏移 0，字符列表位于偏移 24，文字四边形列表位于偏移 48，置信度位于偏移 72。OCRParagraph 为两个 vector，共 48 字节。探针只构造空字符列表，并用置信度字段携带行编号。该工具依赖观察版本的 C++ ABI，不能视作其他版本的通用接口。

```sh
clang++ -std=c++20 -O2 -Wl,-rpath,/Applications/WeChat.app/Contents/Frameworks/ld tools/nativeprobe/paragraph_merge.cpp -o /tmp/paragraph_merge
/tmp/paragraph_merge /Applications/WeChat.app/Contents/Resources/libwxocr.dylib < input.txt
```

输入首行为：`width height native_direction line_count region_count`。原方向值 1～4 对应 Go 方向 0～3。之后每行文字为四组 `x y` 坐标和带双引号的 UTF-8 文字；最后每个段落区域为顶点数量及各顶点 `x y`。输出为按阅读顺序排列的段落，每个段落包含输入行编号。

实际输入和输出在本地 `analysis/paragraph/direct-merge/`；原库 SHA256 与便于 Go 回归的捕获数据在 `testdata/native-cases/native_merge_expected.json`。这些数据验证相同输入下的归并函数行为，不包含原引擎自身的段落区域模型推理。
