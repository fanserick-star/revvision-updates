'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const sourceDir=path.resolve(__dirname,'../ui');
const scripts=['dialogs.js','app.js','commerce_ui.js','receipt_print.js'];
for(const file of scripts){
 const code=fs.readFileSync(path.join(sourceDir,file),'utf8');
 // Compile every browser script, to catch missing awaits/braces before release.
 new vm.Script(code,{filename:file});
 const native=/\\b(?:window\\s*\\.\\s*)?(?:alert|confirm|prompt)\\s*\\(/g;
 assert.equal(native.test(code),false,file+' still contains native popup');
}
const index=fs.readFileSync(path.join(sourceDir,'index.html'),'utf8');
assert.ok(index.includes('src="/dialogs.js"'));
assert.ok(index.indexOf('src="/dialogs.js"')<index.indexOf('src="/app.js"'));
const commerce=fs.readFileSync(path.join(sourceDir,'commerce_ui.js'),'utf8');
const paymentStart=commerce.indexOf("const payment=await api('/payments'");
const confirmationStart=commerce.indexOf("if(await revConfirm({title:'Abono registrado'");
const printStart=commerce.indexOf('await printReceipt(newRow,payment,p,false)');
assert.ok(paymentStart>0&&confirmationStart>paymentStart&&printStart>confirmationStart,'The payment must be persisted before asking to print.');
assert.ok(commerce.includes("cancelText:'Ahora no'")&&commerce.includes("confirmText:'Imprimir comprobante'"));
assert.ok(fs.readFileSync(path.join(sourceDir,'app.js'),'utf8').includes('async function confirmLeave()'));

class FakeElement {
 constructor(name){this.name=name;this.handlers={};this.nodes={};this.attributes={};this.dataset={};this.open=false;this.isConnected=true;this.textContent='';this.classList={add(){}};this.focused=false}
 setAttribute(name,value){this.attributes[name]=value}
 querySelector(selector){return this.nodes[selector]??(this.nodes[selector]=new FakeElement(selector))}
 addEventListener(name,callback){this.handlers[name]=callback}
 fire(name,event={}){this.handlers[name]?.(event)}
 focus(){this.focused=true}
 showModal(){this.open=true}
 close(){this.open=false}
 remove(){this.isConnected=false}
}
const shown=[];
const before=new FakeElement('original-focused-button');
const document={activeElement:before,createElement(tag){assert.equal(tag,'dialog');return new FakeElement('dialog')},body:{appendChild(element){shown.push(element)}}};
const context={window:{},document,Promise,queueMicrotask};
vm.createContext(context);
vm.runInContext(fs.readFileSync(path.join(sourceDir,'dialogs.js'),'utf8'),context);

(async function(){
 const unsafe='<img src=x onerror=alert(1)>';
 const cancelled=context.window.revConfirm({title:'Abono registrado',message:unsafe,confirmText:'Imprimir comprobante',cancelText:'Ahora no',kind:'success'});
 const first=shown.at(-1);
 assert.equal(first.open,true);
 assert.equal(first.querySelector('#rv-dialog-message').textContent,unsafe,'User data must only appear as plain text');
 assert.equal(first.querySelector('.rv-dialog-cancel').focused,true,'Focus defaults to safe action');
 first.querySelector('.rv-dialog-cancel').fire('click');
 assert.equal(await cancelled,false,'Cancel must never trigger printing');
 assert.equal(first.isConnected,false);
 assert.equal(before.focused,true,'Restore focus after modal closes');

 const approved=context.window.revConfirm({title:'Confirmar',confirmText:'Aceptar'});
 const second=shown.at(-1);
 second.querySelector('.rv-dialog-ok').fire('click');
 assert.equal(await approved,true);

 const escape=context.window.revConfirm({message:'Salir sin guardar?',kind:'danger'});
 const third=shown.at(-1);
 let prevented=false;
 third.fire('cancel',{preventDefault(){prevented=true}});
 assert.equal(await escape,false,'Escape must cancel unsafe action');
 assert.equal(prevented,true);

 const one=context.window.revConfirm({message:'Primera'});
 const two=context.window.revConfirm({message:'Segunda'});
 assert.equal(shown.length,4,'Dialog queue must not show two overlays at once');
 shown.at(-1).querySelector('.rv-dialog-ok').fire('click');
 assert.equal(await one,true);
 await Promise.resolve();
 assert.equal(shown.length,5,'Next dialog in queue must open after the previous one');
 shown.at(-1).querySelector('.rv-dialog-x').fire('click');
 assert.equal(await two,false);
 console.log('Dialogs: JS syntax, native-popup audit, safe cancel, payment ordering, Escape, queue, focus and HTML escaping OK');
})().catch(error=>{console.error(error);process.exitCode=1});
