const fs=require('fs'),vm=require('vm'),assert=require('assert');
const code=fs.readFileSync(require('path').join(__dirname,'..','ui','receipt_print.js'),'utf8');
const context={console,money:(v)=>'$'+(Number(v)/100).toFixed(2),esc:(x)=>String(x??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))};
vm.createContext(context);vm.runInContext(code,context);
const row={order:{id:'abcd1234-test',total_cents:10000,lines:[{name:'MONTURA <MODELO>',quantity:1,unit_price_cents:10000,subtotal_cents:10000}]},payments:[
  {id:'a',receipt_number:'RV-01',amount_cents:2000,method:'EFECTIVO',paid_at:'2026-10-17T13:00:00Z'},
  {id:'b',receipt_number:'RV-02',amount_cents:4000,method:'EFECTIVO',paid_at:'2026-10-18T13:00:00Z'},
]};
const patient={full_name:'PRUEBA',document_id:'0912345675'};
for (const fmt of ['80','a4']) {
  const doc=context.receiptHtml(row,row.payments[0],patient,true,fmt);
  assert(doc.includes(fmt==='80'?'size: 80mm':'size: A4'));
  assert(doc.includes('RV-01') && !doc.includes('RV-02'));
  assert(doc.includes('$20.00') && doc.includes('$80.00') && doc.includes('$0.00'));
  assert(doc.includes('REIMPRESIÓN DEL MISMO ABONO'));
  assert(doc.includes('&lt;MODELO&gt;') && !doc.includes('MONTURA <MODELO>'));
  const next=context.receiptHtml(row,row.payments[1],patient,false,fmt);
  assert(next.includes('$40.00') && next.includes('$20.00') && next.includes('$40.00'));
  assert(!next.includes('REIMPRESIÓN DEL MISMO ABONO'));
}
assert.throws(()=>context.receiptHtml(row,{id:'fantasma'},patient,true,'80'),/no se encontró/i);
assert.throws(()=>context.receiptHtml(row,row.payments[0],patient,false,'58'),/no disponible/i);
assert.deepStrictEqual(row.payments.map(x=>x.amount_cents),[2000,4000]);
console.log('OK: formatos aislados; original, reimpresión, saldos y escape HTML. Sin mutación de pagos.');
