// Run with: node template_test.cjs. Tests the classifier actually embedded in HTML.
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const page=fs.readFileSync(__dirname+'/index.html','utf8');
const script=page.match(/<script>([\s\S]*?)<\/script>/)[1];new vm.Script(script);
const classifier=script.slice(script.indexOf('function classifyDocument('),script.indexOf('function renderDocument('));
const context=vm.createContext({});vm.runInContext(classifier,context);
const classify=texts=>context.classifyDocument({lines:texts.map((text,i)=>({text,top:i*30,bottom:i*30+20,left:0,right:200}))});
let doc=classify(['姓名张三','性别男民族汉','出生1990年1月2日','住址北京市朝阳区','公民身份号码110101199001020011']);
assert.equal(doc.type,'id_card');assert.equal(doc.side,'front');assert.equal(doc.fields.name,'张三');assert.equal(doc.fields.sex,'男');assert.equal(doc.fields.ethnicity,'汉');assert.equal(doc.fields.id_number,'110101199001020011');assert.equal(doc.needs_review,false);
doc=classify(['中华人民共和国居民身份证','签发机关北京市公安局','有效期限2020.01.01-2040.01.01']);assert.equal(doc.side,'back');assert.equal(doc.fields.issuing_authority,'北京市公安局');
for(const texts of [[],['人员名单','姓名张三','联系电话123456789'],['姓名张三','性别男','民族汉'],['110101199001020011']])assert.equal(classify(texts).type,'unknown');
doc=context.classifyDocument({lines:[{text:'姓名',left:10,right:50,top:10,bottom:30},{text:'张三',left:65,right:100,top:11,bottom:31},{text:'性别男民族汉',left:0,right:200,top:40,bottom:60},{text:'住址北京市朝阳区',left:10,right:200,top:70,bottom:90},{text:'幸福路一号',left:65,right:160,top:100,bottom:120},{text:'公民身份号码110101199001020011',left:0,right:200,top:140,bottom:160}]});
assert.equal(doc.fields.name,'张三');assert.equal(doc.fields.address,'北京市朝阳区幸福路一号');assert.equal(doc.needs_review,true);
assert(page.indexOf('id="tab-document"')<page.indexOf('id="tab-text"'));
assert(!script.includes('data.document'));assert(script.includes("fetch('./api/ocr?preview=true&mode='"));assert(page.includes('<option value="fast" selected>'));assert(script.includes("encodeURIComponent($('recognition-mode').value)"));
console.log('HTML classifier: front, back, ordinary, separate boxes, missing fields, tab order and script syntax passed');
// Draw every recognized box with the same style and score; leave OCR data intact.
const drawSource=script.slice(script.indexOf('function displayLines('),script.indexOf("$('boxes').onchange"));
const strokes=[],labels=[],mockContext={strokeStyle:'',fillStyle:'',strokeRect(...rect){strokes.push({color:this.strokeStyle,rect});},fillText(text){labels.push(text);}};
const mockCanvas={getContext(){return mockContext;}};
const drawContext=vm.createContext({canvas:mockCanvas,$:()=>({checked:true})});vm.runInContext(drawSource,drawContext);
const sample={width:200,height:100,lines:[{text:'accepted',confidence:.96,left:1,top:20,right:30,bottom:40}],candidates:[{text:'low score',confidence:.23,left:50,top:20,right:80,bottom:40}]};
const before=JSON.stringify(sample);drawContext.draw(sample);
assert.equal(strokes.length,2);assert.equal(new Set(strokes.map(x=>x.color)).size,1);assert.deepEqual(labels,['1 · 0.96','2 · 0.23']);assert.equal(JSON.stringify(sample),before);
assert.equal(drawContext.confidenceText(NaN),'—');assert.equal(drawContext.displayLines({}).length,0);
assert(!page.includes('id="recall-summary"'));assert(!page.includes('低分候选与诊断'));assert(page.includes('>文字框数</span>'));assert(page.includes('>文字框与分数</button>'));
console.log('Unified frame style, score labels, full box list and immutable OCR data passed');
