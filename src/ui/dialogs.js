'use strict';
// Rev Visión: accessible native <dialog> with application branding.
// No browser alert/confirm/prompt and no patient data is stored here.
(function(){
  const pending=[];
  let active=false;
  function next(){
    if(active||!pending.length)return;
    active=true;
    const job=pending.shift(), opts=job.options;
    const dialog=document.createElement('dialog');
    dialog.className='rv-dialog';
    dialog.setAttribute('aria-labelledby','rv-dialog-title');
    dialog.setAttribute('aria-describedby','rv-dialog-message');
    const category=opts.kind==='danger'?'danger':opts.kind==='success'?'success':'question';
    dialog.dataset.kind=category;
    dialog.innerHTML='<div class="rv-dialog-card"><div class="rv-dialog-top"><span class="rv-dialog-symbol" aria-hidden="true"></span><button type="button" class="rv-dialog-x" aria-label="Cerrar aviso">×</button></div><h2 id="rv-dialog-title"></h2><p id="rv-dialog-message"></p><div class="rv-dialog-actions"><button type="button" class="btn rv-dialog-cancel"></button><button type="button" class="btn btn-primary rv-dialog-ok"></button></div></div>';
    dialog.querySelector('.rv-dialog-symbol').textContent=category==='success'?'✓':category==='danger'?'!':'?';
    dialog.querySelector('#rv-dialog-title').textContent=String(opts.title||'Confirmación');
    dialog.querySelector('#rv-dialog-message').textContent=String(opts.message||'');
    const cancel=dialog.querySelector('.rv-dialog-cancel');
    const ok=dialog.querySelector('.rv-dialog-ok');
    cancel.textContent=String(opts.cancelText||'Cancelar');
    ok.textContent=String(opts.confirmText||'Aceptar');
    if(category==='danger'){ok.classList.add('rv-dialog-danger')}
    const previous=document.activeElement;
    let settled=false;
    const finish=(answer)=>{
      if(settled)return;
      settled=true;
      if(dialog.open)dialog.close();
      dialog.remove();
      active=false;
      if(previous&&previous.isConnected&&typeof previous.focus==='function')previous.focus({preventScroll:true});
      job.resolve(answer);
      // Resolve this interaction before starting the next queued question.
      queueMicrotask(next);
    };
    cancel.addEventListener('click',()=>finish(false));
    dialog.querySelector('.rv-dialog-x').addEventListener('click',()=>finish(false));
    ok.addEventListener('click',()=>finish(true));
    dialog.addEventListener('cancel',e=>{e.preventDefault();finish(false)});
    dialog.addEventListener('click',e=>{if(e.target===dialog)finish(false)});
    document.body.appendChild(dialog);
    try{
      dialog.showModal();
      // Focus the safe action by default. Enter cannot silently confirm loss.
      cancel.focus();
    }catch(e){
      finish(false); // Fail closed: never accept a dangerous action implicitly.
    }
  }
  window.revConfirm=function(options){
    const normalized=typeof options==='string'?{message:options}:options||{};
    return new Promise(resolve=>{pending.push({options:normalized,resolve});next()});
  };
})();
