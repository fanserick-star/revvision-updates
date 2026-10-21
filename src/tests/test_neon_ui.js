'use strict';
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),assert=require('node:assert/strict');
const ui=path.resolve(__dirname,'../ui');
for(const file of ['app.js','dialogs.js','commerce_ui.js','receipt_print.js','neon_ui.js','connection_status.js']){
 const content=fs.readFileSync(path.join(ui,file),'utf8');
 new vm.Script(content,{filename:file});
}
const html=fs.readFileSync(path.join(ui,'index.html'),'utf8');
for(const name of ['navNeon','rvConnectionState','/neon_ui.js','/connection_status.js'])
 assert.ok(html.includes(name),'Missing Neon/AFK UI '+name);
assert.ok(!html.includes('PRUEBA LOCAL'),'Trial label still visible');
const neon=fs.readFileSync(path.join(ui,'neon_ui.js'),'utf8');
for(const uri of ['/neon/link','/neon/backup/enable','/neon/backup/now','/neon/backup/restore','/neon/backup/status'])
 assert.ok(neon.includes(uri),'Missing server integration '+uri);
assert.ok(neon.includes('He guardado mi llave'),'Recovery key confirmation missing');
const status=fs.readFileSync(path.join(ui,'connection_status.js'),'utf8');
assert.ok(status.includes('8*60*1000')&&status.includes('10*60*1000'));
assert.ok(status.includes("Neon en línea · Guardado local"));
assert.ok(!status.includes('localStorage'),'Neon secret/status must not use localStorage');
console.log('Revvision Neon settings, offline/AFK states and JS compilation OK');
