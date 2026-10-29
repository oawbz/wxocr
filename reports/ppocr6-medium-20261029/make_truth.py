import json,pathlib,hashlib
root=pathlib.Path(__file__).resolve().parents[2]
cn=['微信模型跨平台测试','中文识别 2026','Hello OCR 123']
invoice=['订单编号 AB-20261004-123456','客户名称：跨平台识别测试','合计金额：￥12345.67','短词 OCR 中文 ABC 123','状态：已完成 数量：100','日期 2026-10-04 11:30','地址：北京市朝阳区测试路88号','备注：中文English混合识别测试','服务热线 400-800-1234','电子邮箱 test@example.com','左栏：名称 数量 金额','右栏：客户 联系方式']
cols=lambda n:[f'第{c}栏第{i}行：识别测试。' for c in range(1,n+1) for i in range(1,7)]
stress=['中文小字与英文 Mixed text 2026','订单号 AB-123456 金额 1234.56','地址：北京市朝阳区测试路88号','识别结果需要逐行对照原引擎。','Repeated letters AAABB CCC 001122']
stagger=['跨列标题：复杂排版测试']+[f'左栏 {i}：中文测试。' for i in range(1,6)]+[f'右栏 {i}：English {i}.' for i in range(1,6)]+['跨列页脚：全部内容结束。']
out=[]
for p in sorted((root/'testdata/native-cases').glob('*.png')):
 n=p.stem
 if n=='blank':lines=[]
 elif n.startswith('chinese') or n.startswith('rotated_'):lines=cn
 elif n.startswith('dense_invoice'):lines=invoice
 elif n.startswith('layout_two_columns'):lines=cols(2)
 elif n=='layout_three_columns':lines=cols(3)
 elif n=='layout_title_footer':lines=['跨平台模型对照：多栏文档阅读顺序']+cols(2)+['文档结束：Go 调用与原引擎对照验证。']
 elif n=='long_line':lines=['长行测试：ABCDEFGHIJKLMNOPQRSTUVWXYZ 0123456789 中文模型跨平台 Go OCR 原引擎对照验证，长度超过常规识别宽度。']
 elif n.startswith('stress_staggered'):lines=stagger
 elif n.startswith('stress_'):lines=stress
 else:continue
 out.append(dict(file=p.name,sha256=hashlib.sha256(p.read_bytes()).hexdigest(),lines=lines))
pathlib.Path(__file__).with_name('truth.json').write_text(json.dumps({'source':'Manually transcribed from viewed rendered images; not native_expected.json. All variants inherit the same visible source text.','cases':out},ensure_ascii=False,indent=2))
print('truth cases',len(out),'lines',sum(len(x['lines']) for x in out))
