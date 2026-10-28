(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const state = { current: null, list: [], newSpot: false, providerSyncing: false };

  async function api(path, options) {
    const response = await fetch('/admin/api' + path, Object.assign({ credentials: 'same-origin' }, options || {}));
    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.code && payload.code !== 0) throw new Error(payload.error || ('请求失败：' + response.status));
    return payload.data;
  }
  function toast(message, tone) { if (window.AdminUI) window.AdminUI.alert(message, tone); else window.alert(message); }
  function textCell(value) { const node=document.createElement('div');node.textContent=value||'—';return node; }
  function isManual(spot) { return !!spot.manual_edited || (spot.provider_data_status!=='synced'&&!!(spot.name||spot.category||spot.address||spot.description||(spot.photos||[]).length)); }
  function modeLabel(spot) { return isManual(spot) ? '人工' : '自动'; }
  function setModeBadge(spot) { const badge=$('detailMode');badge.textContent=modeLabel(spot);badge.className='scenic-mode '+(isManual(spot)?'manual':'automatic'); }
  function photoURL(url) { return (url||'').startsWith('/api/v1/static/')?url.replace('/api/v1/static/','/admin/api/static/'):url; }

  async function loadStats() {
    try {
      const data=await api('/scenic-spots/statistics');$('statsPeriod').textContent=data.from_day+' 至 '+data.to_day;
      $('statsPublished').textContent=(data.published_spots||0).toLocaleString()+' / '+(data.total_spots||0).toLocaleString();
      $('statsPublishedNote').textContent='已发布 / 全部景点';
      $('statsCoveredTracks').textContent=(data.published_linked_tracks||0).toLocaleString()+' / '+(data.linked_tracks||0).toLocaleString();
      $('statsCoveredNote').textContent='发布景点覆盖轨迹 / 已关联轨迹';
      $('statsTodayCalls').textContent=(data.calls_today||0).toLocaleString();$('statsMonthCalls').textContent=(data.calls_30_days||0).toLocaleString();
      $('statsMonthNote').textContent='成功 '+(data.successes_30_days||0).toLocaleString()+' · 失败 '+(data.failures_30_days||0).toLocaleString();
      const daily=data.daily||[],max=Math.max(1,...daily.map(item=>item.requests||0)),bars=$('statsDailyBars');bars.replaceChildren();
      daily.forEach(item=>{const col=document.createElement('div');col.className='scenic-bar-col';const bar=document.createElement('div');bar.className='scenic-bar';bar.style.height=Math.max(2,((item.requests||0)/max)*100)+'%';bar.title=item.day+'：'+(item.requests||0)+' 次';col.appendChild(bar);bars.appendChild(col);});
      $('statsDailyLabels').textContent=daily.length?daily[0].day+'　—　'+daily[daily.length-1].day:'暂无数据';
      const host=$('statsTopSpots');host.replaceChildren();const top=(data.top_spots||[]).slice(0,3);
      if(!top.length){host.textContent='暂无调用记录';}
      top.forEach((item,index)=>{const row=document.createElement('div');row.className='scenic-top-item';row.dataset.spotId=item.scenic_spot_id;const medal=document.createElement('span');medal.className='scenic-medal';medal.textContent=['🥇','🥈','🥉'][index];const name=document.createElement('strong');name.textContent=item.name||'待补充名称';const count=document.createElement('span');count.className='scenic-hint';count.textContent=(item.requests||0).toLocaleString()+' 次';row.append(medal,name,count);row.addEventListener('click',()=>loadSpot(item.scenic_spot_id));host.appendChild(row);});
    } catch(error) { $('statsTopSpots').textContent='统计加载失败：'+error.message; }
  }

  function renderList() {
    const mode=$('statusFilter').value,query=$('searchInput').value.trim().toLocaleLowerCase();
    const spots=state.list.filter(spot=>(!mode||(mode==='manual')===isManual(spot))&&(!query||(spot.name||'').toLocaleLowerCase().includes(query)));
    $('spotCount').textContent='共 '+spots.length+' 处';const host=$('spotRows');host.replaceChildren();
    if(!spots.length){const empty=document.createElement('div');empty.className='p-3 scenic-hint';empty.textContent='没有匹配的景点';host.appendChild(empty);return;}
    spots.forEach(spot=>{const row=document.createElement('article');row.className='scenic-row'+(state.current&&state.current.id===spot.id?' active':'');row.dataset.spotId=spot.id;row.tabIndex=0;
      const main=document.createElement('div');main.className='scenic-row-main';const title=document.createElement('div');title.className='scenic-row-title';const titleText=document.createElement('span');titleText.className='scenic-row-title-text';titleText.textContent=spot.name||'待补充名称';const badge=document.createElement('span');badge.className='scenic-mode '+(isManual(spot)?'manual':'automatic');badge.textContent=modeLabel(spot);title.append(titleText,badge);
      const sub=document.createElement('div');sub.className='scenic-row-sub';if(spot.category){const category=document.createElement('span');category.className='scenic-hint';category.textContent=spot.category;sub.appendChild(category);}const city=spot.city_name||spot.city_code;if(city){const cityText=document.createElement('span');cityText.className='scenic-hint scenic-city';cityText.textContent=city;sub.appendChild(cityText);}
      const description=document.createElement('div');description.className='scenic-row-description';description.textContent=spot.description||spot.address||'暂无简介';main.append(title,sub,description);
      const images=document.createElement('div');images.className='scenic-row-photos';(spot.photos||[]).slice(0,3).forEach(photo=>{const img=document.createElement('img');img.loading='lazy';img.src=photoURL(photo.url);img.alt=photo.caption||spot.name||'景点图片';images.appendChild(img);});row.append(main,images);row.addEventListener('click',()=>loadSpot(spot.id));row.addEventListener('keydown',event=>{if(event.key==='Enter')loadSpot(spot.id);});host.appendChild(row);
    });
  }
  async function loadList() {
    try{const params=new URLSearchParams({q:$('searchInput').value.trim(),limit:'100'});const data=await api('/scenic-spots?'+params);state.list=data.items||[];renderList();}
    catch(error){toast(error.message,'danger');}
  }

  async function syncProviderContent() {
    if(state.providerSyncing)return;state.providerSyncing=true;const progress=$('providerSyncProgress');progress.hidden=false;
    let cursor='',scanned=0,synced=0,noSource=0,failed=0;
    try{
      while(true){const params=new URLSearchParams({limit:'5'});if(cursor)params.set('cursor',cursor);const result=await api('/scenic-spots/provider-content/backfill?'+params,{method:'POST'});scanned+=result.scanned||0;synced+=result.synced||0;noSource+=result.no_source||0;failed+=result.failed||0;progress.textContent='供应商资料同步：已扫描 '+scanned+' 处，已入库 '+synced+' 处，缺少来源 '+noSource+' 处，失败 '+failed+' 处';if(!result.has_more)break;cursor=result.next_cursor||'';if(!cursor)break;}
      await Promise.all([loadList(),loadStats()]);progress.textContent='供应商资料同步完成：本轮入库 '+synced+' 处，缺少来源 '+noSource+' 处，失败 '+failed+' 处。';
    }catch(error){progress.textContent='供应商资料同步中断：'+error.message;}
    finally{state.providerSyncing=false;}
  }

  function fillForm(spot) {
    state.current=spot;$('detailModal').showModal();$('detailTitle').textContent=state.newSpot?'新建景点':(spot.name||'待补充名称');
    $('detailMeta').textContent=spot.id?'内部 ID：'+spot.id:'保存后生成内部景点 ID';setModeBadge(spot);
    $('nameInput').value=spot.name||'';$('categoryInput').value=spot.category||'';$('addressInput').value=spot.address||'';$('descriptionInput').value=spot.description||'';
    const cities=spot.related_cities||[];$('cityInfo').textContent=cities.length?'城市：'+cities.join('、'):(spot.city_name||spot.city_code)?'城市：'+(spot.city_name||spot.city_code):'城市：暂无';
    $('providerNameHint').textContent='';$('providerCategoryHint').textContent='';$('providerAddressHint').textContent='';$('providerDescriptionHint').textContent='';$('photosBox').replaceChildren();(spot.photos||[]).forEach(addPhotoCard);
    $('publishBtn').disabled=state.newSpot||spot.status==='merged'||spot.status==='archived';$('publishBtn').textContent=spot.status==='published'?'取消发布':'发布';$('uploadPhotoBtn').disabled=state.newSpot;
  }
  async function loadSpot(id) {
    try{const spot=await api('/scenic-spots/'+encodeURIComponent(id));state.newSpot=false;fillForm(spot);document.querySelectorAll('.scenic-row').forEach(row=>row.classList.toggle('active',row.dataset.spotId===id));renderList();}
    catch(error){toast(error.message,'danger');}
  }

  function moveDraggedPhoto(target,event){const dragging=$('photosBox').querySelector('.scenic-photo.dragging');if(!dragging||dragging===target)return;const rect=target.getBoundingClientRect();const after=event.clientY>rect.top+rect.height/2;$('photosBox').insertBefore(dragging,after?target.nextSibling:target);}
  function addPhotoCard(photo) {
    const card=document.createElement('div');card.className='scenic-photo';card.draggable=true;const img=document.createElement('img');img.src=photoURL(photo.url);img.alt=photo.alt_text||photo.caption||'景点图片';
    const caption=document.createElement('input');caption.className='form-control';caption.placeholder='图片说明';caption.value=photo.caption||'';
    const alt=document.createElement('input');alt.className='form-control';alt.placeholder='替代文本';alt.value=photo.alt_text||'';
    const attr=document.createElement('input');attr.className='form-control';attr.placeholder='来源/授权署名';attr.value=photo.attribution||'';
    const controls=document.createElement('div');controls.className='d-flex gap-2 mt-2';const remove=document.createElement('button');remove.type='button';remove.className='btn btn-sm btn-outline-danger ms-auto';remove.textContent='移除';remove.addEventListener('click',()=>card.remove());controls.appendChild(remove);
    card.dataset.photoId=photo.id;card.dataset.url=photo.url;card.dataset.provider=photo.provider||'';card.dataset.sourceUrl=photo.source_url||'';card.dataset.attributionUrl=photo.attribution_url||'';
    if(photo.provider){const source=document.createElement('div');source.className='scenic-photo-source';source.textContent=photo.provider==='google'?'Google':'高德';card.appendChild(source);}
    if(photo.source_url){const link=document.createElement('a');link.href=photo.source_url;link.target='_blank';link.rel='noopener noreferrer';link.textContent='图片来源';link.className='small d-block mt-1';card.appendChild(link);}
    card.append(img,caption,alt,attr,controls);$('photosBox').appendChild(card);
    card.addEventListener('dragstart',event=>{if(event.target.closest('input,button,a')){event.preventDefault();return;}card.classList.add('dragging');event.dataTransfer.effectAllowed='move';});
    card.addEventListener('dragend',()=>{card.classList.remove('dragging');$('photosBox').querySelectorAll('.drop-target').forEach(item=>item.classList.remove('drop-target'));});
    card.addEventListener('dragover',event=>{event.preventDefault();if(!card.classList.contains('dragging')){card.classList.add('drop-target');moveDraggedPhoto(card,event);}});
    card.addEventListener('dragleave',()=>card.classList.remove('drop-target'));
    card.addEventListener('drop',event=>{event.preventDefault();card.classList.remove('drop-target');moveDraggedPhoto(card,event);});
  }
  function currentPayload() {
    const base=state.current||{};const photos=Array.from($('photosBox').children).map(card=>{const inputs=card.querySelectorAll('input');return{id:card.dataset.photoId,url:card.dataset.url,caption:inputs[0].value.trim(),alt_text:inputs[1].value.trim(),attribution:inputs[2].value.trim(),attribution_url:card.dataset.attributionUrl||undefined,source_url:card.dataset.sourceUrl||undefined,provider:card.dataset.provider||undefined};});
    return Object.assign({},base,{name:$('nameInput').value.trim(),category:$('categoryInput').value.trim(),address:$('addressInput').value.trim(),description:$('descriptionInput').value.trim(),photos});
  }
  async function save() {
    try{const body=currentPayload();let spot;if(state.newSpot){spot=await api('/scenic-spots',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});}else{spot=await api('/scenic-spots/'+encodeURIComponent(state.current.id),{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({expected_version:state.current.version,spot:body})});}
      state.newSpot=false;fillForm(spot);await Promise.all([loadList(),loadStats()]);toast('景点已保存','success');
    }catch(error){toast(error.message,'danger');}
  }
  async function setStatus(status) {
    if(!state.current)return;try{const spot=await api('/scenic-spots/'+encodeURIComponent(state.current.id)+'/status',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({status,expected_version:state.current.version})});state.newSpot=false;fillForm(spot);await Promise.all([loadList(),loadStats()]);toast(status==='published'?'已发布':'已取消发布','success');}
    catch(error){toast(error.message,'danger');}
  }
  async function uploadPhoto() {
    if(!state.current)return;const file=$('photoFile').files[0];if(!file){toast('先选择 JPG 或 PNG 图片','danger');return;}const form=new FormData();form.append('file',file);form.append('caption',$('photoCaption').value.trim());
    try{const data=await api('/scenic-spots/'+encodeURIComponent(state.current.id)+'/photos',{method:'POST',body:form});state.current=data.spot;if(data.photo)addPhotoCard(data.photo);$('detailMeta').textContent='内部 ID：'+state.current.id;$('photoFile').value='';$('photoCaption').value='';toast('图片已添加','success');}
    catch(error){toast(error.message,'danger');}
  }

  $('searchBtn').addEventListener('click',loadList);$('searchInput').addEventListener('keydown',event=>{if(event.key==='Enter')loadList();});$('statusFilter').addEventListener('change',renderList);$('refreshStatsBtn').addEventListener('click',loadStats);
  $('newBtn').addEventListener('click',()=>{state.newSpot=true;fillForm({status:'draft',manual_edited:true});});$('closeDetailBtn').addEventListener('click',()=>$('detailModal').close());$('detailModal').addEventListener('click',event=>{if(event.target===$('detailModal'))$('detailModal').close();});
  $('saveBtn').addEventListener('click',save);$('publishBtn').addEventListener('click',()=>setStatus(state.current&&state.current.status==='published'?'draft':'published'));$('uploadPhotoBtn').addEventListener('click',uploadPhoto);
  loadList();loadStats();syncProviderContent();
})();
