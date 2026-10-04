
身份证模板：API 在原有 `result` 外返回 `document`，以 `type: id_card`、`side: front/back` 和 `fields` 表示证件类型及字段。网页自动显示结构化字段，未检测到身份证时继续显示普通 OCR 结果。模板按标签和相邻坐标提取内容，缺失字段返回 `missing_fields` 并设置 `needs_review`，原始文字始终保留。已验证虚构正反面样例，真实拍照证件仍需样本验证。
