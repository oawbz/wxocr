# 20260713122533.jpg 构建与识别对比

本次优化在该图上保留了 42 条此前被过滤的候选；正文文字与优化前完全一致，尚未修复其正文漏识别和错字。候选不等于新增正确内容，仍需核对原图。

## 构建与测试条件

- 构建：macOS arm64，完整发布包位于 `/Users/awbz/code/微信 ocr/examples/web/dist/darwin-arm64-20261029-142844-30484/`，含程序、模型、运行库和配置。
- 优化前：从提交 `2cfde64` 提取源码到独立临时目录构建；优化后：当前工作区。两者使用相同本地模型和 ONNX Runtime。
- 输入：用户提供的原始 JPEG，2480×3506。识别直接使用原图，未手动旋转或增强。
- 每个版本先预热一次，再各运行 3 次；最终计时分开运行，包含解码和 OCR，不含 HTTP、预览生成、JSON 编码与网络传输。耗时是本机样本，不是稳定性能承诺。
- 发布包实测：临时启动本地 HTTP 服务，上传原图，HTTP 200，返回 148 行正文、42 条候选、194 个检测框诊断；测试服务已停止。

## 对比

| 指标 | 优化前 | 优化后 |
|---|---:|---:|
| 正文文字框 | 148 | 148 |
| 可核对候选 | 未返回 | 42 |
| 逐框诊断 | 未返回 | 194 |
| 处理耗时中位数 | 2.54 秒 | 2.77 秒 |

耗时增加约 9.0%。正文按阅读顺序逐条比较，文字完全一致。行数为 OCR 框数，不是表格记录数。

## 实际收益与问题

- 候选中找回了多处“无”、日期的“07”“01”片段，以及“员3”“员9”等姓名末尾；这些内容先前主要因检测尺度面积小于 300 被过滤。
- 候选也含低质量结果，例如“R”“N”“技果文程”，不能直接并入正文。
- 原图长宽比约 1.41，低于长图分块阈值 2，因此该图只有整图检测，没有触发重叠分块。诊断中的 `pass` 均为 0。
- 部分小序号没有形成检测框，候选保留无法恢复此类遗漏；身份证号仍存在错字或多字，部分日期仍被拆碎，底部多行仍有合框和漏识别问题。
- 本次没有完整逐字标注真值，因此不提供字符准确率或提升百分比；42 条候选不能视为 42 条正确补全。

后续方向：为高分辨率普通页面增加局部检测，结合表格行列或文本方向拆分多行框，并在合并时优先使用完整、清晰的局部结果。需以本图和现有旋转、表格语料一起回归，避免重复拼接。

## 识别框对照

仅为方便查看，将下列展示图旋转为正向；识别输入仍为原始 JPEG。青色为正文框，橙色为候选框。

优化前：

![优化前正文框](/Users/awbz/code/微信 ocr/reports/recall-comparison-20261029/before-boxes.png)

优化后：

![优化后正文与候选框](/Users/awbz/code/微信 ocr/reports/recall-comparison-20261029/after-boxes.png)

## 原始结果

- [优化前正文](/Users/awbz/code/微信 ocr/reports/recall-comparison-20261029/before.txt)
- [优化后正文](/Users/awbz/code/微信 ocr/reports/recall-comparison-20261029/after.txt)
- [候选及过滤原因](/Users/awbz/code/微信 ocr/reports/recall-comparison-20261029/candidates.txt)
- [指标汇总](/Users/awbz/code/微信 ocr/reports/recall-comparison-20261029/summary.json)
- [优化前完整测试输出](/Users/awbz/code/微信 ocr/reports/recall-comparison-20261029/before.json)
- [优化后完整测试输出](/Users/awbz/code/微信 ocr/reports/recall-comparison-20261029/after.json)
- [发布包 HTTP 实测响应](/Users/awbz/code/微信 ocr/reports/recall-comparison-20261029/packaged-api.json)
