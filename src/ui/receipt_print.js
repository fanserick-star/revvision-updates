'use strict';
// Recibos de abonos: documento de impresión AUTÓNOMO, sin CSS A4 de historias clínicas.
// El formato no altera ningún abono guardado ni su número de comprobante.
function receiptHtml(row, payment, patient, isCopy, format = '80') {
  if (!row || !row.order || !payment || !patient) throw Error('No hay datos del abono para imprimir');
  if (format !== '80' && format !== 'a4') throw Error('Formato de impresión no disponible');
  const payments = Array.isArray(row.payments) ? row.payments : [];
  const index = payments.findIndex(x => x.id === payment.id);
  if (index < 0) throw Error('No se encontró el abono original en el pedido');
  const previous = payments.slice(0, index).reduce((sum, item) => sum + Number(item.amount_cents), 0);
  const remaining = Number(row.order.total_cents) - previous - Number(payment.amount_cents);
  const order = row.order;
  const total = money(order.total_cents);
  const prefix = `${esc(payment.receipt_number)} · ${esc(patient.full_name)}`;
  const lines = order.lines.map(item => ({
    name: `${esc(item.name)} × ${Number(item.quantity)}`,
    subtotal: money(item.subtotal_cents),
    price: money(item.unit_price_cents),
    qty: Number(item.quantity)
  }));
  const stamp = esc(new Date(payment.paid_at).toLocaleString('es-EC'));
  const common = `
    <p><b>Paciente:</b> ${esc(patient.full_name)}<br><b>Identificación:</b> ${esc(patient.document_id || '—')}<br><b>Pedido:</b> ${esc(order.id.slice(0,8).toUpperCase())}<br><b>Fecha del abono:</b> ${stamp}</p>`;
  // Comprobante térmico deliberadamente compacto: solo datos necesarios, sin espacios de hoja A4.
  const thermalIdentity = `<p><b>Paciente:</b> ${esc(patient.full_name)}<br><b>C.I.:</b> ${esc(patient.document_id || '—')}<br><b>Fecha:</b> ${stamp}</p>`;
  const totals = `
    <div class="total-line"><span>Total del pedido</span><b>${total}</b></div>
    <div class="total-line"><span>Abonado anteriormente</span><b>${money(previous)}</b></div>
    <div class="total-line emphasis"><span>ABONO RECIBIDO</span><b>${money(payment.amount_cents)}</b></div>
    <div class="total-line"><span>Saldo pendiente</span><b>${money(remaining)}</b></div>`;
  const note = `<p class="method"><b>Forma de pago:</b> ${esc(payment.method)}${payment.note ? `<br><b>Referencia:</b> ${esc(payment.note)}` : ''}</p>`;
  const warning = `NO ES FACTURA TRIBUTARIA${isCopy ? '<br>REIMPRESIÓN DEL MISMO ABONO' : ''}`;
  const inner = format === '80' ? `
    <div id="receipt" class="ticket">
      <img src="/revvision-logo.png" class="logo" alt="Rev Visión">
      <h1>COMPROBANTE DE ABONO</h1>
      <p class="center disclaimer">${warning}</p>
      <p><b>N.º:</b> ${esc(payment.receipt_number)}<br><b>Pedido:</b> ${esc(order.id.slice(0,8).toUpperCase())}</p>
      ${thermalIdentity}
      <div class="rule"></div>
      ${lines.map(x => `<div class="item"><span>${x.name}</span><b>${x.subtotal}</b></div>`).join('')}
      <div class="rule"></div>
      ${totals}${note}
      <p class="center thanks">Rev Visión · Dinero recibido</p>
    </div>` : `
    <div id="receipt" class="sheet">
      <div class="mast"><img src="/revvision-logo.png" class="logo" alt="Rev Visión"><div class="company">ÓPTICA Y SALUD VISUAL</div></div>
      <div class="heading"><div><div class="eyebrow">CONTROL DE PEDIDOS</div><h1>COMPROBANTE DE ABONO</h1></div><div class="number"><b>N.º ${esc(payment.receipt_number)}</b><br>${stamp}</div></div>
      <div class="identity">${common}</div>
      <table><thead><tr><th>Artículo</th><th>Cant.</th><th>Precio unit.</th><th>Importe</th></tr></thead><tbody>
      ${lines.map(x => `<tr><td>${x.name.replace(/ × \d+$/, '')}</td><td>${x.qty}</td><td>${x.price}</td><td>${x.subtotal}</td></tr>`).join('')}
      </tbody></table>
      <div class="totals">${totals}</div>${note}
      <div class="footer"><div>${warning}</div><p>Rev Visión · Óptica y Salud Visual<br>Comprobante de dinero recibido</p></div>
    </div>`;
  const style = format === '80' ? `
    /* Tamaño provisional CORTO: el alto definitivo se calcula ANTES de abrir el diálogo. */
    @page { size: 80mm 90mm; margin: 0; }
    *{box-sizing:border-box}
    html,body{margin:0!important;padding:0!important;width:80mm!important;min-width:0!important;max-width:80mm!important;height:auto!important;background:#fff;color:#111;font-family:Arial,sans-serif}
    #receipt{margin:0;width:80mm;max-width:80mm;padding:1.5mm 3.5mm 1mm;break-inside:avoid;font:10.5px/1.2 Arial,sans-serif;overflow-wrap:anywhere}
    h1{text-align:center;font-size:11px;margin:2px 0 3px;line-height:1.2}
    p{margin:3px 0}img.logo{display:block;width:39mm;height:auto;max-height:11mm;max-width:100%;margin:0 auto 2px;object-fit:contain}
    .center{text-align:center}.disclaimer{font-size:8.5px;line-height:1.22;margin:2px 0 5px}.rule{border-top:1px dashed #555;margin:4px 0}
    .item,.total-line{display:flex;justify-content:space-between;gap:5px;margin:2.5px 0;align-items:flex-start}
    .item span,.total-line span{min-width:0;flex:1;overflow-wrap:anywhere}
    .item b,.total-line b{white-space:nowrap;flex-shrink:0;text-align:right}
    .emphasis{border-top:1px solid #222;padding-top:3px;font-size:11px;font-weight:bold}
    .thanks{font-size:9px;margin-top:5px}.method{overflow-wrap:anywhere}
    @media print { html,body{-webkit-print-color-adjust:exact;print-color-adjust:exact} }
  ` : `
    @page { size: A4 portrait; margin: 14mm; }
    *{box-sizing:border-box}html,body{margin:0;padding:0;background:white;color:#17121b;font:12px/1.55 Arial,sans-serif}
    #receipt{max-width:182mm;margin:0 auto;padding:0}
    img.logo{width:80mm;max-height:25mm;object-fit:contain;object-position:left center}
    .mast{display:flex;justify-content:space-between;align-items:center;border-bottom:2px solid #7b3a8e;padding-bottom:6mm;margin-bottom:8mm}
    .company{letter-spacing:1px;font-size:10px;color:#72417d}
    .heading{display:flex;justify-content:space-between;gap:12mm;align-items:start}
    .eyebrow{color:#874292;font-size:10px;letter-spacing:1.5px;font-weight:bold}
    h1{font-size:22px;margin:2mm 0 5mm;color:#271735}
    .number{text-align:right;white-space:nowrap;font-size:11px}
    .identity{border:1px solid #e1d2e5;border-radius:7px;background:#fbf7fc;padding:4mm 5mm;margin:4mm 0 7mm}.identity p{margin:0}
    table{border-collapse:collapse;width:100%;margin:3mm 0 8mm;table-layout:fixed}th,td{text-align:left;border-bottom:1px solid #ddd;padding:9px 7px;overflow-wrap:anywhere}
    th{background:#f4eaf7;color:#4d2861}th:first-child{width:49%}th:not(:first-child),td:not(:first-child){text-align:right}
    .totals{width:88mm;max-width:100%;margin-left:auto}
    .total-line{display:flex;justify-content:space-between;gap:8mm;padding:7px 0;border-bottom:1px solid #e6dfe8}
    .total-line b{white-space:nowrap}.emphasis{font-size:15px;border-top:2px solid #75418b;color:#522664}
    .method{margin-top:11mm}.footer{text-align:center;border-top:1px solid #ddd;padding-top:6mm;margin-top:18mm;color:#554a60;font-size:10px}.footer div{font-weight:bold;color:#202020}
    @media print {html,body{-webkit-print-color-adjust:exact;print-color-adjust:exact}}
  `;
  return `<!DOCTYPE html><html lang="es"><head><meta charset="utf-8"><title>${prefix} · Rev Visión</title><style id="receipt-page-style">${style}</style></head><body>${inner}</body></html>`;
}

// Aísla la impresión del @page A4 global de la Historia Clínica.
// No llama a una API comercial ni registra nuevos pagos.
function openReceiptPrintFrame(markup, format, receiptLabel) {
  document.querySelector('#revvision-receipt-print-frame')?.remove();
  const frame = document.createElement('iframe');
  frame.id = 'revvision-receipt-print-frame';
  frame.title = receiptLabel;
  // CRÍTICO: A4 se prepara a ancho A4; ticket se prepara a 80 mm.
  // Nunca usar un iframe de 1200px de alto: en algunos controladores genera página excesiva.
  frame.style.cssText = `position:fixed;left:-10000px;top:0;width:${format==='80'?'80mm':'210mm'};height:${format==='80'?'90mm':'297mm'};border:0;pointer-events:none`;
  document.body.appendChild(frame);
  frame.onload = async () => {
    try {
      const doc = frame.contentDocument;
      const receipt = doc?.getElementById('receipt');
      if (!receipt) throw Error('No se pudo preparar el comprobante');
      await Promise.all(Array.from(doc.images).map(image => image.complete ? Promise.resolve() : new Promise(resolve => {
        image.onload = resolve; image.onerror = resolve; setTimeout(resolve, 3000);
      })));
      if (doc.fonts?.ready) await doc.fonts.ready;
      if (format === '80') {
        // Medir el contenido ya cargado, incluido el logo. CSS 96 px = 25,4 mm.
        const contentPx = Math.max(receipt.scrollHeight, receipt.getBoundingClientRect().height);
        const contentMM = contentPx * 25.4 / 96;
        const paperHeight = Math.max(40, Math.ceil(contentMM + 2)); // solo 2 mm de cierre
        // Un ticket desproporcionado se bloquea antes de desperdiciar un rollo entero.
        if (paperHeight > 240) throw Error('El ticket es inusualmente largo. Revisa la cantidad de artículos del pedido');
        const pageStyle = doc.getElementById('receipt-page-style');
        if (!pageStyle?.textContent?.includes('size: 80mm 90mm')) throw Error('No se pudo configurar el ancho térmico');
        pageStyle.textContent = pageStyle.textContent.replace('size: 80mm 90mm', `size: 80mm ${paperHeight}mm`);
        frame.style.height = `${paperHeight}mm`;
        // Permitir al navegador recomputar la página antes de imprimir.
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      }
      const view = frame.contentWindow;
      if (!view) throw Error('No se pudo abrir la impresión');
      view.addEventListener('afterprint', () => setTimeout(() => frame.remove(), 500), {once:true});
      view.focus();
      view.print();
    } catch (err) { frame.remove(); msg('No se pudo preparar la impresión: '+err.message, true); }
  };
  frame.srcdoc = markup;
}
// Configuración local: se elige expresamente la impresora instalada en Windows.
let revvisionPrinters=[];
function chosenThermalPrinter(){
  const select=document.getElementById('thermalPrinter');
  return select ? select.value : (localStorage.getItem('revvision-thermal-printer') || '');
}
async function refreshThermalPrinters(){
  const select=document.getElementById('thermalPrinter');
  if(!select)return;
  select.disabled=true;
  select.innerHTML='<option value="">Buscando impresoras de Windows…</option>';
  try{
    const response=await api('/printers');
    revvisionPrinters=Array.isArray(response.printers)?response.printers:[];
    select.innerHTML='<option value="">Seleccionar impresora térmica…</option>'+
      revvisionPrinters.map(p=>`<option value="${esc(p)}">${esc(p)}</option>`).join('');
    const stored=localStorage.getItem('revvision-thermal-printer')||'';
    const storedMatch=revvisionPrinters.find(x=>x===stored);
    const candidates=revvisionPrinters.filter(x=>/pos[-_ ]?80|crm[-_ ]?308|crome/i.test(x));
    select.value=storedMatch||((candidates.length===1)?candidates[0]:'');
    select.onchange=()=>localStorage.setItem('revvision-thermal-printer',select.value);
    select.disabled=false;
    if(!revvisionPrinters.length)msg('Windows no informa ninguna impresora instalada.',true);
  }catch(err){
    select.innerHTML='<option value="">Impresora no disponible</option>';
    msg(err.message||'No se pudieron consultar impresoras',true);
  }
}
function addThermalPrinterSelector(){
  const btn=document.getElementById('thermalPaperTest');
  if(!btn||document.getElementById('thermalPrinter'))return;
  const host=document.createElement('div');
  host.id='thermalPrinterChoice';
  host.className='row wrap';
  host.style.cssText='justify-content:flex-end;align-items:center;gap:8px;flex:1';
  host.innerHTML='<label for="thermalPrinter">Impresora térmica USB:</label><select id="thermalPrinter" class="btn" style="max-width:270px;min-width:180px"></select>';
  btn.before(host);
  const format=document.getElementById('receiptWidth');
  const update=()=>{
    const visible=receiptPaperFormat==='80';
    host.style.display=visible?'flex':'none';
    btn.style.display=visible?'inline-flex':'none';
    const hint=btn.parentElement?.nextElementSibling;
    if(hint?.classList.contains('muted')){
      hint.innerHTML=visible?'El ticket de 80 mm se envía directamente a Windows/ESC-POS, sin diálogo de Edge. Revisa la impresora seleccionada antes de imprimir.':'A4 utiliza el diálogo de impresión de Edge.';
    }
  };
  if(format){format.addEventListener('change',update)}
  update();
  refreshThermalPrinters();
}
async function printReceipt(row, payment, patient, isCopy) {
  const format=receiptPaperFormat === 'a4'?'a4':'80';
  if(format==='a4'){
    try{
      const markup=receiptHtml(row,payment,patient,isCopy,'a4');
      openReceiptPrintFrame(markup, 'a4', 'Comprobante A4');
    }catch(err){msg(err.message,true)}
    return;
  }
  const printer=chosenThermalPrinter();
  if(!printer){msg('Selecciona la impresora POS-80C / CRM-308 antes de imprimir.',true);return}
  if(!revvisionPrinters.includes(printer)){
    msg('La impresora seleccionada no está confirmada por Windows. Actualiza la lista.',true);return;
  }
  try{
    await api('/print/thermal/receipt','POST',{printer,payment_id:payment.id,copy:!!isCopy});
    msg('Ticket enviado a la cola de '+printer+'. Comprueba el corte y la salida física.');
  }catch(err){msg('No se imprimió: '+err.message,true)}
}
async function printThermalTest(){
  const printer=chosenThermalPrinter();
  if(!printer||!revvisionPrinters.includes(printer)){msg('Selecciona la impresora POS-80C / CRM-308 de Windows.',true);return}
  if(!confirm('¿Enviar una PRUEBA CORTA directamente a '+printer+'? No registra abonos.'))return;
  try{await api('/print/thermal/test','POST',{printer});msg('Prueba corta enviada a '+printer+'. Confirma si imprimió y cortó.')}catch(err){msg('No se pudo imprimir la prueba: '+err.message,true)}
}
