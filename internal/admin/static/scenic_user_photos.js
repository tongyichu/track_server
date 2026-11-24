const ScenicUserPhotos = (() => {
  const el = id => document.getElementById(id);
  let spot, cursor, request;
  async function load(append) {
    request?.abort(); const current = request = new AbortController();
    const base = '/admin/api/scenic-spots/' + encodeURIComponent(spot) + '/user-photos';
    try {
      const res = await fetch(base + '?cursor=' + encodeURIComponent(append ? cursor : ''),{signal:current.signal});
      if (!res.ok) throw Error('用户实拍加载失败');
      const {data} = await res.json(); if(current !== request) return;
      if(!append) el('userScenicItems').replaceChildren();
      for(const p of data.items) {
        const box = document.createElement('div'); box.className='d-flex flex-column';
        const image = new Image(112,84); image.loading='lazy'; image.style.objectFit='cover'; image.src=base+'/'+p.id+'/image'; image.alt=p.included?'已收录实拍':'候选实拍';
        const action=document.createElement('button'); action.className='btn btn-sm btn-link'; action.type='button'; action.dataset.adminWrite='scenic_spots';
        action.textContent=p.included?'从景点移除':'收录到景点'; const selectedSpot=spot;
        action.onclick=async () => {
          action.disabled=true;
          try {const res=await fetch(base+'/'+p.id,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({revision:p.revision,scenic_choice:p.included?'-':selectedSpot})});
            if(!res.ok) throw Error('保存失败，请刷新后重试'); if(spot===selectedSpot) load(false);
          } catch(e){if(spot===selectedSpot)el('userScenicMessage').textContent=e.message;action.disabled=false;}
        };
        box.append(image,action); el('userScenicItems').append(box);
      }
      cursor=data.next_cursor || '';el('userScenicMore').hidden=!cursor;
      el('userScenicMessage').textContent=el('userScenicItems').childElementCount||cursor?'':'暂无已审核的用户实拍';
    } catch(e){if(!current.signal.aborted) el('userScenicMessage').textContent=e.message;}
  }
  el('userScenicMore').onclick=()=>load(true);
  return {open(id){request?.abort();spot=id;cursor='';el('userScenicPhotos').hidden=!id;el('userScenicItems').replaceChildren();el('userScenicMore').hidden=true;el('userScenicMessage').textContent='';if(id)load(false);}};
})();