(function () {
  const body = document.getElementById('musicList'), error = document.getElementById('musicError');
  const modal = document.getElementById('musicEditor'), form = document.getElementById('musicForm');
  const save = document.getElementById('musicSave'), saveError = document.getElementById('musicSaveError');
  let current = null, controller = null, version = 0, saving = false, saveController = null;
  function text(tag, value, className) { const el = document.createElement(tag); el.textContent = value; if(className) el.className=className; return el; }
  async function request(url, options) { const response=await fetch(url,options); const data=await response.json().catch(()=>({})); if(!response.ok) throw new Error(data.error || '读取失败（'+response.status+'）'); return data.data; }
  function edit(item) {
    current=item; form.reset();saveError.hidden=true;
    document.getElementById('musicEditTitle').textContent=item?'编辑音乐':'添加音乐';
    ['name','artist','license','source','attribution','sort_order'].forEach(key=>{form.elements[key].value=item?item[key]:key==='sort_order'?'0':'';});
    form.elements.published.checked=Boolean(item&&item.published); form.elements.audio.required=!item;
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
        row.append(info,text('td',Math.floor(item.duration_ms/60000)+':'+String(Math.floor(item.duration_ms/1000)%60).padStart(2,'0')+' / '+(item.audio_bytes/1048576).toFixed(1)+'MB'),text('td',item.license||'未填写'),text('td',item.sort_order),text('td',item.published?'已上架':'已下架'));
        const actions=document.createElement('td'),button=text('button','编辑','btn btn-sm btn-outline-primary');button.type='button';button.dataset.adminWrite='music';button.addEventListener('click',()=>edit(item));actions.append(button);row.append(actions);body.append(row);
      });
      if(!data.items.length){const row=document.createElement('tr'),cell=text('td','暂无音乐，点击“添加音乐”上传。');cell.colSpan=6;row.append(cell);body.append(row);}
    } catch(e) { if(run===version){error.textContent=e.name==='AbortError'?'读取超时，请刷新重试。':e.message;error.hidden=false;} }
    finally {clearTimeout(timeout);if(run===version)document.getElementById('musicRefresh').disabled=false;}
  }
  form.addEventListener('submit',async e=>{
    e.preventDefault();if(saving||!AdminAccess.can('music',true))return;saving=true;save.disabled=true;saveError.hidden=true;
    const item={id:current?current.id:'',revision:current?current.revision:0};['name','artist','license','source','attribution'].forEach(key=>{item[key]=form.elements[key].value.trim();});item.sort_order=Number(form.elements.sort_order.value);item.published=form.elements.published.checked;
    const data=new FormData();data.set('metadata',JSON.stringify(item));['audio','cover'].forEach(key=>{const f=form.elements[key].files[0];if(f)data.set(key,f);});
    saveController=new AbortController();const timeout=setTimeout(()=>saveController.abort(),60000);
    try {await request('/admin/api/music'+(current?'/'+encodeURIComponent(current.id):''),{method:current?'PUT':'POST',body:data,signal:saveController.signal});modal.close();await load();}
    catch(e){saveError.textContent=e.name==='AbortError'?'保存超时，请刷新确认结果后再操作。':e.message;saveError.hidden=false;}
    finally{clearTimeout(timeout);saveController=null;saving=false;save.disabled=false;}
  });
  document.getElementById('musicClose').onclick=()=>{if(!saving)modal.close();};modal.addEventListener('cancel',e=>{if(saving)e.preventDefault();});
  document.getElementById('musicAdd').onclick=()=>edit(null);document.getElementById('musicRefresh').onclick=load;
  window.addEventListener('pagehide',()=>{version++;if(controller)controller.abort();if(saveController)saveController.abort();body.querySelectorAll('audio').forEach(el=>el.pause());});
  document.addEventListener('visibilitychange',()=>{if(document.hidden)body.querySelectorAll('audio').forEach(el=>el.pause());});load();
})();
