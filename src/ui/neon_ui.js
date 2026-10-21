'use strict';
// Revvision's optional cloud settings; never put PostgreSQL credentials in
// localStorage, URLs, logs or page markup. All secrets go to local Go only.
async function rvNeonScreen(){
 if(!await confirmLeave())return;
 screen='neon';setNav('neon');
 app.innerHTML=heading('Respaldo opcional','Nube Neon',`<button type="button" class="btn" id="rvNeonHome">← Inicio</button>`)+`
 <div class="panel rv-neon">
 <h2>Conectar cuenta Neon de esta clínica</h2>
 <p class="muted">Revvision guarda pacientes y consultas en esta PC, incluso sin Internet. Para tener una copia remota cifrada, crea un proyecto propio en <a href="https://console.neon.tech/" target="_blank" rel="noopener noreferrer">Neon</a>. Nunca reutilices una cuenta de otra clínica.</p>
 <p id="rvNeonInfo" class="muted" role="status">Leyendo configuración local…</p>
 <div class="field"><label for="rvNeonURL">Cadena PostgreSQL Neon con sslmode=require</label><input id="rvNeonURL" type="password" autocomplete="new-password" spellcheck="false" placeholder="postgresql://usuario:••••@ep-…neon.tech/neondb?sslmode=require"></div>
 <div class="row wrap">
 <button class="btn" type="button" id="rvNeonTest">Probar conexión</button>
 <button class="btn btn-primary" type="button" id="rvNeonLink">Vincular mi cuenta</button>
 <button class="btn" type="button" id="rvNeonCheck">Comprobar Neon</button>
 <button class="btn btn-danger" type="button" id="rvNeonUnlink">Desvincular esta PC</button>
 </div>
 <div class="rv-neon-separator"></div>
 <h2>Respaldos cifrados y ahorro AFK</h2>
 <p class="muted">Las copias usan AES-256-GCM antes de salir de la PC. Con actividad, se intentan solo si los datos cambiaron y con intervalos de 15 minutos; tras 8 minutos sin usar Revvision se pausan. No sincronizan varias PCs ni mezclan fichas.</p>
 <p id="rvBackupInfo" class="muted" role="status">Comprobando último respaldo…</p>
 <div class="row wrap">
 <button class="btn btn-primary" type="button" id="rvBackupEnable">Activar respaldo cifrado</button>
 <button class="btn" type="button" id="rvBackupNow">Respaldar ahora</button>
 <button class="btn" type="button" id="rvBackupKey">Ver llave de esta PC</button>
 </div>
 <section id="rvKeyBox" class="rv-keybox" hidden><strong>Llave de recuperación</strong><p>Guárdala fuera de esta PC; sin ella no podrás recuperar una copia después de una avería.</p><textarea id="rvKeyText" rows="2" readonly spellcheck="false"></textarea><div class="row wrap"><button type="button" class="btn" id="rvCopyKey">Copiar llave</button><button type="button" class="btn btn-primary" id="rvConfirmKey">He guardado mi llave</button></div></section>
 <details class="rv-recovery"><summary>Recuperar en una computadora nueva y vacía</summary>
 <p class="muted">Necesitas la URL del proyecto Neon y tu llave de recuperación. No se permitirá restaurar sobre pacientes, consultas ni ventas existentes.</p>
 <div class="field"><label for="rvRestoreURL">Cadena Neon original</label><input id="rvRestoreURL" type="password" autocomplete="off"></div>
 <div class="field"><label for="rvRestoreKey">Llave de recuperación</label><input id="rvRestoreKey" type="password" autocomplete="off"></div>
 <button type="button" class="btn btn-danger" id="rvRestore">Recuperar en esta PC vacía</button>
 </details>
 <p class="muted rv-neon-note">La nube es un respaldo opcional, no reemplaza controles de acceso, cifrado del disco ni pruebas de recuperación con tu proyecto real. Las copias locales diarias se guardan además en la carpeta recovery del programa.</p>
 </div>`;
 const root=app;root.querySelector('#rvNeonHome').onclick=home;
 const allButtons=[...root.querySelectorAll('button')];
 let linked=false,backed=false;
 const el=id=>root.querySelector('#'+id);
 const busy=async fn=>{allButtons.forEach(b=>b.disabled=true);try{await fn()}catch(e){msg(e.message||String(e),true)}finally{allButtons.forEach(b=>b.disabled=false);await refresh().catch(()=>{})}};
 async function refresh(){
  const [n,b]=await Promise.all([api('/neon/status'),api('/neon/backup/status')]);
  linked=!!n.linked;backed=!!b.ready;
  el('rvNeonInfo').textContent=linked?'Vinculada: '+n.database+' · '+n.host+' (datos locales prioritarios)':'Neon sin vincular · guardado local disponible';
  let status=b.ready?'Respaldo cifrado habilitado':b.enabled?'Llave creada: confirma que la guardaste':'Respaldo remoto desactivado';
  if(b.last_backup)status+=' · Último respaldo confirmado: '+new Date(b.last_backup).toLocaleString('es-EC');
  else status+=' · Todavía no hay copia confirmada en Neon';
  if(b.last_error)status+=' · Último error: '+b.last_error;
  el('rvBackupInfo').textContent=status;
  el('rvBackupEnable').disabled=!!b.enabled||!linked;
  el('rvBackupNow').disabled=!b.ready||!linked;
  el('rvBackupKey').disabled=!b.enabled;
  window.rvRefreshConnection?.();
 }
 const showKey=k=>{
  el('rvKeyText').value=k;
  el('rvKeyBox').hidden=false;
  el('rvKeyText').focus();
 };
 el('rvCopyKey').onclick=async()=>{
  try{await navigator.clipboard.writeText(el('rvKeyText').value);msg('Llave copiada. Guárdala fuera de esta PC.')}catch{el('rvKeyText').select();msg('Selecciona y copia la llave manualmente.')}
 };
 el('rvConfirmKey').onclick=()=>busy(async()=>{
  if(!await revConfirm({title:'Confirmar llave guardada',message:'¿Guardaste realmente la llave fuera de esta computadora? Sin ella no podrás descifrar la copia de Neon.',confirmText:'Sí, la guardé',cancelText:'Todavía no'}))return;
  await api('/neon/backup/confirm','POST');
  el('rvKeyText').value='';el('rvKeyBox').hidden=true;msg('Respaldo cifrado habilitado')
 });
 el('rvNeonTest').onclick=()=>busy(async()=>{
  const v=el('rvNeonURL').value;if(!v){msg('Pega primero la URL de Neon',true);return}
  const x=await api('/neon/test','POST',{connection_url:v});msg('Conexión correcta: '+x.database)
 });
 el('rvNeonLink').onclick=()=>busy(async()=>{
  const v=el('rvNeonURL').value;if(!v){msg('Pega primero la URL de Neon',true);return}
  if(linked&&!await revConfirm({title:'Cambiar cuenta Neon',message:'¿Deseas vincular una cuenta distinta? Los respaldos anteriores no se borran de Neon.',confirmText:'Continuar',cancelText:'Cancelar',kind:'danger'}))return;
  const x=await api('/neon/link','POST',{connection_url:v});el('rvNeonURL').value='';msg(x.message)
 });
 el('rvNeonCheck').onclick=()=>busy(async()=>{await api('/neon/check','POST');msg('Neon respondió correctamente')});
 el('rvNeonUnlink').onclick=()=>busy(async()=>{
  if(!linked)return;
  if(!await revConfirm({title:'Desvincular Neon',message:'Se desconectará esta PC. Los respaldos ya existentes permanecerán en tu proyecto Neon. Los datos locales no se modifican.',confirmText:'Desvincular',cancelText:'Cancelar',kind:'danger'}))return;
  await api('/neon/unlink','POST');msg('Neon desvinculado de esta PC')
 });
 el('rvBackupEnable').onclick=()=>busy(async()=>{
  if(!await revConfirm({title:'Activar respaldo cifrado',message:'La llave se mostrará una sola vez al crearla. Guarda una copia fuera de esta PC antes de permitir el primer respaldo.',confirmText:'Crear llave',cancelText:'Cancelar'}))return;
  const x=await api('/neon/backup/enable','POST');showKey(x.recovery_key);msg('Guarda y confirma la llave de recuperación')
 });
 el('rvBackupKey').onclick=()=>busy(async()=>{const x=await api('/neon/backup/key','POST');showKey(x.recovery_key)});
 el('rvBackupNow').onclick=()=>busy(async()=>{
  const x=await api('/neon/backup/now','POST');msg(x.uploaded?'Neon confirmó el respaldo cifrado':'No hay cambios desde la última copia')
 });
 el('rvRestore').onclick=()=>busy(async()=>{
  const url=el('rvRestoreURL').value,key=el('rvRestoreKey').value;
  if(!url||!key){msg('Ingresa conexión Neon y llave de recuperación',true);return}
  if(!await revConfirm({title:'Restaurar respaldo',message:'La restauración solo es válida en una instalación completamente vacía. No mezclar ni sobrescribir historiales existentes.',confirmText:'Comprobar y restaurar',cancelText:'Cancelar',kind:'danger'}))return;
  const x=await api('/neon/backup/restore','POST',{connection_url:url,recovery_key:key});
  el('rvRestoreURL').value='';el('rvRestoreKey').value='';msg(x.message)
 });
 await refresh();
}
