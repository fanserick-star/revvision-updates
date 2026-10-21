'use strict';
// Status is informational: Neon online != backed up. Local save always works.
(function(){
 const badge=document.getElementById('rvConnectionState');
 if(!badge)return;
 let lastInteraction=Date.now(),lastRemote=0,lastPulse=0,linked=false,busy=false;
 const timeout=8*60*1000,checkEvery=10*60*1000;
 function display(text,mode){badge.textContent=text;badge.dataset.state=mode;badge.title='Guardado local primero. Estado de Neon no equivale a respaldo confirmado.'}
 async function pulse(){
  if(Date.now()-lastPulse<60*1000)return;
  lastPulse=Date.now();
  try{await fetch('/api/neon/activity',{method:'POST',headers:{'X-RevVision-Client':'revvision-local','Content-Type':'application/json'}})}catch{}
 }
 function touch(){
  const idle=Date.now()-lastInteraction>timeout;
  lastInteraction=Date.now();void pulse();
  if(idle)void assess(true);
 }
 for(const event of ['pointerdown','keydown','input']){window.addEventListener(event,touch,{passive:true})}
 document.addEventListener('visibilitychange',()=>{if(!document.hidden){touch();void assess(true)}});
 window.addEventListener('online',()=>{touch();void assess(true)});
 window.addEventListener('offline',()=>display('Sin Internet · Guardado local','offline'));
 async function assess(force=false){
  if(!navigator.onLine){display('Sin Internet · Guardado local','offline');return}
  if(document.hidden||Date.now()-lastInteraction>timeout){display('AFK · Ahorro Neon','afk');return}
  if(busy||(!force&&Date.now()-lastRemote<checkEvery))return;
  busy=true;lastRemote=Date.now();
  try{
   const config=await fetch('/api/neon/status',{cache:'no-store'}).then(r=>{if(!r.ok)throw Error();return r.json()});
   linked=!!config.linked;
   if(!linked){display('Guardado local · Neon sin vincular','local');return}
   const ctl=new AbortController();const timer=setTimeout(()=>ctl.abort(),12000);
   try{
    const reply=await fetch('/api/neon/check',{method:'POST',headers:{'Content-Type':'application/json','X-RevVision-Client':'revvision-local'},signal:ctl.signal});
    if(!reply.ok)throw Error('Sin conexión a Neon');
    display('Neon en línea · Guardado local','online');
   }finally{clearTimeout(timer)}
  }catch{
   display(linked?'Neon no disponible · Guardado local':'Guardado local · Neon sin vincular',linked?'offline':'local');
  }finally{busy=false}
 }
 void pulse();void assess(true);
 setInterval(()=>{
  if(document.hidden||Date.now()-lastInteraction>timeout){display('AFK · Ahorro Neon','afk');return}
  void pulse();void assess(false);
 },60*1000);
 window.rvRefreshConnection=()=>{lastRemote=0;void assess(true)};
})();
