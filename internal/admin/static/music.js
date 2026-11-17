(function () {
  const body = document.getElementById('musicList'), error = document.getElementById('musicError');
  const modal = document.getElementById('musicEditor'), form = document.getElementById('musicForm');
  const save = document.getElementById('musicSave'), saveError = document.getElementById('musicSaveError');
  let current = null, controller = null, version = 0, saving = false, saveController = null;
  function text(tag, value, className) { const el = document.createElement(tag); el.textContent = value; if(className) el.className=className; return el; }
  async function request(url, options) { const response=await fetch(url,options); const data=await response.json().catch(()=>({})); if(!response.ok) throw new Error(data.error || '读取失败（'+response.status+'）'); return data.data; }
  const previewBox=document.getElementById('musicPreviewBox'), previewRoot=document.getElementById('musicPreviewPlayer');
  const previewState=document.getElementById('musicPreviewState');
  let preview=null, previewObjectURL=null, previewVersion=0;
  function stopPreview() {
    previewVersion++;
    if(preview){preview.pause();preview.removeAttribute('src');preview.load();preview.remove();preview=null;}
    if(previewObjectURL){URL.revokeObjectURL(previewObjectURL);previewObjectURL=null;}
    previewBox.hidden=true;
  }
  function showPreview() {
    stopPreview(); const run=previewVersion;
    let url='';
    if(form.elements.audio_mode.value==='link') {
      const raw=form.elements.audio_link.value.trim(); if(!raw)return;
      try {const parsed=new URL(raw);if(!['http:','https:'].includes(parsed.protocol)||parsed.username||parsed.password)throw new Error();url=parsed.href;}
      catch(_){previewBox.hidden=false;previewState.textContent='请填写有效的 HTTP/HTTPS 音频链接。';return;}
    }else{
      const file=form.elements.audio.files[0];
      if(file){previewObjectURL=URL.createObjectURL(file);url=previewObjectURL;}
      else if(current&&!current.audio_link)url=current.audio_url;
    }
    if(!url)return;
    const player=document.createElement('audio');preview=player;player.controls=true;player.preload='none';player.src=url;
    previewRoot.append(player);previewBox.hidden=false;previewState.textContent='点击播放，确认能正常出声。';
    player.addEventListener('loadedmetadata',()=>{
      if(run!==previewVersion)return;
      const duration=player.duration;
      if(Number.isFinite(duration)&&duration>=1&&duration<=1200&&form.elements.audio_mode.value==='link')form.elements.duration_seconds.value=String(Math.round(duration*1000)/1000);
    });
    player.addEventListener('playing',()=>{if(run===previewVersion){previewState.textContent='正在播放，可以确认声音后保存。';body.querySelectorAll('audio').forEach(other=>other.pause());}});
    player.addEventListener('error',()=>{if(run===previewVersion)previewState.textContent='暂时无法播放，请检查直链是否有效或被原站限制。';});
  }
  function audioMode() {
    const link=form.elements.audio_mode.value==='link', writable=AdminAccess.can('music',true);
    document.getElementById('musicFileField').hidden=link;
    document.getElementById('musicLinkField').hidden=!link;
    document.getElementById('musicDurationField').hidden=!link;
    form.elements.audio.disabled=link||!writable;form.elements.audio.required=!link&&(!current||Boolean(current.audio_link));
    form.elements.audio_link.disabled=!link||!writable;form.elements.audio_link.required=link;
    form.elements.duration_seconds.disabled=!link||!writable;
    showPreview();
  }
  form.elements.audio_mode.addEventListener('change',audioMode);
  form.elements.audio_link.addEventListener('input',()=>{form.elements.duration_seconds.value='';showPreview();});
  form.elements.audio.addEventListener('change',showPreview);
  modal.addEventListener('close',stopPreview);
  function edit(item) {
    current=item; body.querySelectorAll('audio').forEach(player=>player.pause());form.reset();saveError.hidden=true;
    document.getElementById('musicEditTitle').textContent=item?'编辑音乐':'添加音乐';
    ['name','artist','license','source','attribution','sort_order'].forEach(key=>{form.elements[key].value=item?item[key]:key==='sort_order'?'0':'';});
    form.elements.published.checked=Boolean(item&&item.published);
    form.elements.audio_mode.value=item&&item.audio_link?'link':'upload';
    form.elements.audio_link.value=item&&item.audio_link||'';
    form.elements.duration_seconds.value=item&&item.audio_link&&item.duration_ms>0?String(item.duration_ms/1000):'';
    audioMode();
    modal.showModal();
  }
  async function load() {
    if(controller) controller.abort();controller=new AbortController();const active=controller;const run=++version;
    error.hidden=true;document.getElementById('musicRefresh').disabled=true;const timeout=setTimeout(()=>active.abort(),12000);
    try {
      const data=await request('/admin/api/music',{signal:active.signal});if(run!==version)return;
      body.querySelectorAll('audio').forEach(el=>el.pause());body.replaceChildren();
      data.items.forEach(item=>{
        const row=document.createElement('tr'), info=document.createElement('td'), box=text('div','','music-cell');
        if(item.cover_url){const img=document.createElement('img');img.src=item.cover_url;img.alt='';box.append(img);}
        const name=document.createElement('div');name.append(text('strong',item.name),text('small',item.artist));box.append(name);info.append(box);
        const audio=document.createElement('audio');audio.controls=true;audio.preload='none';audio.src=item.audio_url;audio.className='music-preview';audio.addEventListener('play',()=>body.querySelectorAll('audio').forEach(other=>{if(other!==audio)other.pause();}));info.append(audio);
        row.append(info,text('td',(item.duration_ms>0?Math.floor(item.duration_ms/60000)+':'+String(Math.floor(item.duration_ms/1000)%60).padStart(2,'0'):'时长未知')+' / '+(item.audio_bytes>0?(item.audio_bytes/1048576).toFixed(1)+'MB':'大小未知')),text('td',item.license||'未填写'),text('td',item.sort_order),text('td',item.published?'已上架':'已下架'));
        const actions=document.createElement('td'),button=text('button','编辑','btn btn-sm btn-outline-primary');button.type='button';button.dataset.adminWrite='music';button.addEventListener('click',()=>edit(item));actions.append(button);row.append(actions);body.append(row);
      });
      if(!data.items.length){const row=document.createElement('tr'),cell=text('td','暂无音乐，点击“添加音乐”上传。');cell.colSpan=6;row.append(cell);body.append(row);}
    } catch(e) { if(run===version){error.textContent=e.name==='AbortError'?'读取超时，请刷新重试。':e.message;error.hidden=false;} }
    finally {clearTimeout(timeout);if(run===version)document.getElementById('musicRefresh').disabled=false;}
  }
  form.addEventListener('submit',async e=>{
    e.preventDefault();if(saving||!AdminAccess.can('music',true))return;saving=true;save.disabled=true;saveError.hidden=true;
    const item={id:current?current.id:'',revision:current?current.revision:0};['name','artist','license','source','attribution'].forEach(key=>{item[key]=form.elements[key].value.trim();});item.sort_order=Number(form.elements.sort_order.value);item.published=form.elements.published.checked;
    const linked=form.elements.audio_mode.value==='link';item.audio_link=linked?form.elements.audio_link.value.trim():'';
    item.duration_ms=linked?Math.round(Number(form.elements.duration_seconds.value||0)*1000):0;
    const data=new FormData();data.set('metadata',JSON.stringify(item));['audio','cover'].forEach(key=>{const f=form.elements[key].files[0];if(f&&(key!=='audio'||!linked))data.set(key,f);});
    saveController=new AbortController();const timeout=setTimeout(()=>saveController.abort(),60000);
    try {await request('/admin/api/music'+(current?'/'+encodeURIComponent(current.id):''),{method:current?'PUT':'POST',body:data,signal:saveController.signal});modal.close();await load();}
    catch(e){saveError.textContent=e.name==='AbortError'?'保存超时，请刷新确认结果后再操作。':e.message;saveError.hidden=false;}
    finally{clearTimeout(timeout);saveController=null;saving=false;save.disabled=false;}
  });
  document.getElementById('musicClose').onclick=()=>{if(!saving)modal.close();};modal.addEventListener('cancel',e=>{if(saving)e.preventDefault();});
  document.getElementById('musicAdd').onclick=()=>edit(null);document.getElementById('musicRefresh').onclick=load;
  window.addEventListener('pagehide',()=>{stopPreview();version++;if(controller)controller.abort();if(saveController)saveController.abort();body.querySelectorAll('audio').forEach(el=>el.pause());});
  document.addEventListener('visibilitychange',()=>{if(document.hidden){if(preview)preview.pause();body.querySelectorAll('audio').forEach(el=>el.pause());}});load();
})();
