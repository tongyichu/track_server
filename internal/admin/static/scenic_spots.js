(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const state = { current: null, list: [], newSpot: false, providerSyncing: false, pendingCoordinates: null, map: null, mapMarker: null, mapOriginalPosition: null, selectingMapLocation: false, mapSDKPromise: null, photoPreviewURL: null };

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
  function spotCities(spot) { const cities=spot.related_cities&&spot.related_cities.length?spot.related_cities:[spot.city_name].filter(Boolean);return [...new Set(cities)]; }

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
    const mode=$('statusFilter').value,city=$('cityFilter').value,query=$('searchInput').value.trim().toLocaleLowerCase();
    const spots=state.list.filter(spot=>(!mode||(mode==='manual')===isManual(spot))&&(!city||spotCities(spot).includes(city))&&(!query||(spot.name||'').toLocaleLowerCase().includes(query)));
    $('spotCount').textContent='共 '+spots.length+' 处';const host=$('spotRows');host.replaceChildren();
    if(!spots.length){const empty=document.createElement('tr');empty.innerHTML='<td colspan="6" class="p-3 scenic-hint">没有匹配的景点</td>';host.appendChild(empty);return;}
    spots.forEach(spot=>{const row=document.createElement('tr');row.dataset.spotId=spot.id;
      const cityCell=document.createElement('td');cityCell.className='city-cell';cityCell.textContent=spotCities(spot).join('、')||'—';
      const nameCell=document.createElement('td');nameCell.className='name-cell';nameCell.textContent=spot.name||'待补充名称';
      const categoryCell=document.createElement('td');categoryCell.className='category-cell';categoryCell.textContent=spot.category||'—';categoryCell.title=spot.category||'';
      const modeCell=document.createElement('td');const badge=document.createElement('span');badge.className='scenic-mode '+(isManual(spot)?'manual':'automatic');badge.textContent=modeLabel(spot);modeCell.appendChild(badge);
      const imageCell=document.createElement('td');const images=document.createElement('div');images.className='scenic-row-photos';(spot.photos||[]).slice(0,3).forEach(photo=>{const img=document.createElement('img');img.loading='lazy';img.src=photoURL(photo.url);img.alt=photo.caption||spot.name||'景点图片';images.appendChild(img);});if(!images.children.length){const hint=document.createElement('span');hint.className='scenic-hint';hint.textContent='暂无图片';images.appendChild(hint);}imageCell.appendChild(images);
      const actionCell=document.createElement('td');const view=document.createElement('button');view.type='button';view.className='btn btn-sm btn-outline-primary';view.textContent='查看详情';view.addEventListener('click',()=>loadSpot(spot.id));actionCell.appendChild(view);
      row.append(cityCell,nameCell,categoryCell,modeCell,imageCell,actionCell);host.appendChild(row);
    });
  }
  function updateCityOptions() { const select=$('cityFilter'),current=select.value,cities=[...new Set(state.list.flatMap(spotCities))].sort((a,b)=>a.localeCompare(b,'zh-CN'));select.replaceChildren(new Option('全部城市',''));cities.forEach(city=>select.add(new Option(city,city)));if(cities.includes(current))select.value=current; }
  async function loadList() {
    try{const params=new URLSearchParams({q:$('searchInput').value.trim(),limit:'100'});const data=await api('/scenic-spots?'+params);state.list=data.items||[];updateCityOptions();renderList();}
    catch(error){toast(error.message,'danger');}
  }

  async function syncProviderContent() {
    if(state.providerSyncing)return;state.providerSyncing=true;const progress=$('providerSyncProgress');progress.hidden=false;
    let cursor='',scanned=0,synced=0,coordinates=0,noSource=0,failed=0;
    try{
      while(true){const params=new URLSearchParams({limit:'5'});if(cursor)params.set('cursor',cursor);const result=await api('/scenic-spots/provider-content/backfill?'+params,{method:'POST'});scanned+=result.scanned||0;synced+=result.synced||0;coordinates+=result.coordinates||0;noSource+=result.no_source||0;failed+=result.failed||0;progress.textContent='供应商资料同步：已扫描 '+scanned+' 处，新增资料 '+synced+' 处，补齐坐标 '+coordinates+' 处，缺少来源 '+noSource+' 处，失败 '+failed+' 处';if(!result.has_more)break;cursor=result.next_cursor||'';if(!cursor)break;}
      await Promise.all([loadList(),loadStats()]);progress.textContent='供应商资料同步完成：新增资料 '+synced+' 处，补齐坐标 '+coordinates+' 处，缺少来源 '+noSource+' 处，失败 '+failed+' 处。';
    }catch(error){progress.textContent='供应商资料同步中断：'+error.message;}
    finally{state.providerSyncing=false;}
  }

  function fillForm(spot) {
    state.current=spot;$('detailModal').showModal();$('detailTitle').textContent=state.newSpot?'新建景点':(spot.name||'待补充名称');
    $('detailMeta').textContent=spot.id?'内部 ID：'+spot.id:'保存后生成内部景点 ID';setModeBadge(spot);
    $('nameInput').value=spot.name||'';$('categoryInput').value=spot.category||'';$('addressInput').value=spot.address||'';$('descriptionInput').value=spot.description||'';
    const cities=spotCities(spot);$('cityInfo').textContent=cities.length?cities.join('、'):'暂无';
    state.pendingCoordinates=null;state.selectingMapLocation=false;const lat=Number(spot.latitude),lng=Number(spot.longitude),hasCoordinates=Number.isFinite(lat)&&Number.isFinite(lng)&&lat!==0&&lng!==0;$('coordinateInfo').hidden=!hasCoordinates;$('coordinateInfo').textContent=hasCoordinates?'坐标（'+(spot.coordinate_system||'未知坐标系')+'）：'+lat.toFixed(6)+'，'+lng.toFixed(6):'';$('coordinateMap').hidden=!spot.id;$('coordinateMapError').hidden=true;$('coordinateMapHelp').hidden=!spot.id;$('coordinateMapActions').hidden=!spot.id;$('chooseMapLocationBtn').disabled=!spot.id;$('chooseMapLocationBtn').textContent=hasCoordinates?'重新选点':'选择位置';$('resetMapLocationBtn').disabled=true;$('coordinateMapActionHint').textContent='地图点击仅用于浏览；选点请先点击“'+$('chooseMapLocationBtn').textContent+'”。';$('coordinateMapImage').hidden=!hasCoordinates;
    if(hasCoordinates&&spot.id){$('coordinateMapImage').src='/admin/api/scenic-spots/'+encodeURIComponent(spot.id)+'/map?v='+encodeURIComponent(spot.version||'');}else{$('coordinateMapImage').removeAttribute('src');}$('copySpotIdBtn').disabled=!spot.id;
    if(spot.id)initializeScenicMap(spot);
    $('providerNameHint').textContent='';$('providerCategoryHint').textContent='';$('providerAddressHint').textContent='';$('providerDescriptionHint').textContent='';clearPhotoPreview();renderPhotoCards(spot.photos||[]);
  }
  function toGCJ02(lat,lng,system){if(system!=='WGS84')return{lat,lng};const inside=lng>=72.004&&lng<=137.8347&&lat>=0.8293&&lat<=55.8271;if(!inside)return{lat,lng};let x=lng-105,y=lat-35,a=-100+2*x+3*y+.2*y*y+.1*x*y+.2*Math.sqrt(Math.abs(x));a+=(20*Math.sin(6*x*Math.PI)+20*Math.sin(2*x*Math.PI))*2/3;a+=(20*Math.sin(y*Math.PI)+40*Math.sin(y/3*Math.PI))*2/3;a+=(160*Math.sin(y/12*Math.PI)+320*Math.sin(y*Math.PI/30))*2/3;let b=300+x+2*y+.1*x*x+.1*x*y+.1*Math.sqrt(Math.abs(x));b+=(20*Math.sin(6*x*Math.PI)+20*Math.sin(2*x*Math.PI))*2/3;b+=(20*Math.sin(x*Math.PI)+40*Math.sin(x/3*Math.PI))*2/3;b+=(150*Math.sin(x/12*Math.PI)+300*Math.sin(x/30*Math.PI))*2/3;const rad=lat/180*Math.PI,magic=1-.00669342162296594323*Math.sin(rad)**2,root=Math.sqrt(magic);return{lat:lat+a*180/((6378245*(1-.00669342162296594323))/(magic*root)*Math.PI),lng:lng+b*180/(6378245/root*Math.cos(rad)*Math.PI)};}
  function loadAMapSDK(config){if(window.AMap)return Promise.resolve(window.AMap);if(state.mapSDKPromise)return state.mapSDKPromise;state.mapSDKPromise=new Promise((resolve,reject)=>{window._AMapSecurityConfig={serviceHost:window.location.origin+config.service_host};const script=document.createElement('script');script.src='https://webapi.amap.com/maps?v=2.0&key='+encodeURIComponent(config.key)+'&plugin=AMap.Scale,AMap.ToolBar';script.onload=()=>window.AMap?resolve(window.AMap):reject(new Error('高德地图 SDK 未就绪'));script.onerror=()=>reject(new Error('高德地图 SDK 加载失败'));document.head.appendChild(script);});return state.mapSDKPromise;}
  function setMapSelectionMode(enabled){state.selectingMapLocation=!!enabled;$('coordinateMapCanvas').classList.toggle('selecting-location',state.selectingMapLocation);const hasSavedPosition=!!state.mapOriginalPosition?.hasCoordinates;$('chooseMapLocationBtn').textContent=state.selectingMapLocation?'取消选点':(hasSavedPosition?'重新选点':'选择位置');$('coordinateMapActionHint').textContent=state.selectingMapLocation?'请在地图上点击新位置；拖动地图可先浏览到目标区域。':'地图点击仅用于浏览；选点请先点击“'+$('chooseMapLocationBtn').textContent+'”。';}
  function updateMapMarker(lat,lng){if(!state.mapMarker){state.mapMarker=new window.AMap.Marker({position:[lng,lat],draggable:false});state.map.add(state.mapMarker);}else{state.mapMarker.setPosition([lng,lat]);}}
  async function initializeScenicMap(spot){if(state.map){state.map.destroy();state.map=null;state.mapMarker=null;}state.mapOriginalPosition=null;state.selectingMapLocation=false;$('coordinateMapError').hidden=true;$('coordinateMapHelp').hidden=false;try{const config=await api('/scenic-spots/map-config');if(!config.configured){$('coordinateMapError').textContent='缺少高德 Web(JS API) Key 或安全密钥';$('coordinateMapError').hidden=false;$('chooseMapLocationBtn').disabled=true;$('coordinateMapActionHint').textContent='地图服务未配置，暂时无法选点。';return;}const AMap=await loadAMapSDK(config);if(state.current?.id!==spot.id)return;const lat=Number(spot.latitude),lng=Number(spot.longitude),hasCoordinates=Number.isFinite(lat)&&Number.isFinite(lng)&&lat!==0&&lng!==0;const center=hasCoordinates?toGCJ02(lat,lng,spot.coordinate_system):{lat:35.8617,lng:104.1954};const zoom=hasCoordinates?15:5;state.mapOriginalPosition={latitude:lat,longitude:lng,coordinateSystem:spot.coordinate_system||'',hasCoordinates,mapLat:center.lat,mapLng:center.lng,zoom};window.AMap=AMap;state.map=new AMap.Map('coordinateMapCanvas',{zoom,center:[center.lng,center.lat],resizeEnable:true,scrollWheel:true});state.map.addControl(new AMap.Scale());state.map.addControl(new AMap.ToolBar({position:{right:'10px',top:'10px'}}));if(hasCoordinates){state.mapMarker=new AMap.Marker({position:[center.lng,center.lat],draggable:false});state.map.add(state.mapMarker);}state.map.on('click',event=>{if(!state.selectingMapLocation)return;const position=event.lnglat;updateMapMarker(position.getLat(),position.getLng());setPendingCoordinates(position.getLat(),position.getLng());$('resetMapLocationBtn').disabled=false;setMapSelectionMode(false);});$('chooseMapLocationBtn').disabled=false;setMapSelectionMode(false);}catch(error){$('coordinateMapError').textContent=error.message||'地图暂时无法加载';$('coordinateMapError').hidden=false;$('chooseMapLocationBtn').disabled=true;}}
  function setPendingCoordinates(lat,lng){state.pendingCoordinates={latitude:Number(lat),longitude:Number(lng),coordinate_system:'GCJ02'};$('coordinateInfo').hidden=false;$('coordinateInfo').textContent='坐标（GCJ02）：'+state.pendingCoordinates.latitude.toFixed(6)+'，'+state.pendingCoordinates.longitude.toFixed(6)+' · 未保存';}
  function resetMapLocation(){if(!state.map||!state.mapOriginalPosition)return;const original=state.mapOriginalPosition;state.pendingCoordinates=null;setMapSelectionMode(false);if(original.hasCoordinates){updateMapMarker(original.mapLat,original.mapLng);state.map.setZoom(original.zoom);state.map.setCenter([original.mapLng,original.mapLat]);$('coordinateInfo').hidden=false;$('coordinateInfo').textContent='坐标（'+(original.coordinateSystem||'未知坐标系')+'）：'+Number(original.latitude).toFixed(6)+'，'+Number(original.longitude).toFixed(6);}else{if(state.mapMarker){state.map.remove(state.mapMarker);state.mapMarker=null;}state.map.setZoom(original.zoom);state.map.setCenter([original.mapLng,original.mapLat]);$('coordinateInfo').hidden=true;$('coordinateInfo').textContent='';}$('resetMapLocationBtn').disabled=true;}
  $('coordinateMapImage').addEventListener('error',()=>{$('coordinateMapImage').hidden=true;$('coordinateMapError').hidden=false;});
  async function loadSpot(id) {
    try{const spot=await api('/scenic-spots/'+encodeURIComponent(id));state.newSpot=false;fillForm(spot);document.querySelectorAll('.scenic-row').forEach(row=>row.classList.toggle('active',row.dataset.spotId===id));renderList();}
    catch(error){toast(error.message,'danger');}
  }

  function clearPhotoPreview(){if(state.photoPreviewURL){URL.revokeObjectURL(state.photoPreviewURL);state.photoPreviewURL=null;}$('photoFile').value='';}
  function moveDraggedPhoto(target,event){const dragging=$('photosBox').querySelector('.scenic-photo-item.dragging');if(!dragging||dragging===target)return;const rect=target.getBoundingClientRect();const after=event.clientY>rect.top+rect.height/2;const addCard=$('photosBox').querySelector('.scenic-photo-add-card');if(target===addCard){$('photosBox').insertBefore(dragging,addCard);return;}const anchor=after?target.nextElementSibling:target;if(anchor==null||anchor===addCard)$('photosBox').insertBefore(dragging,addCard);else $('photosBox').insertBefore(dragging,anchor);}
  function bindPhotoDragEvents(card,draggable){
    card.draggable=draggable;
    card.addEventListener('dragstart',event=>{if(!draggable)return;card.classList.add('dragging');event.dataTransfer.effectAllowed='move';event.dataTransfer.setData('text/plain',card.dataset.photoId||'photo');});
    card.addEventListener('dragend',()=>{card.classList.remove('dragging');$('photosBox').querySelectorAll('.drop-target').forEach(item=>item.classList.remove('drop-target'));});
    card.addEventListener('dragover',event=>{if(!$('photosBox').querySelector('.scenic-photo-item.dragging'))return;event.preventDefault();if(!card.classList.contains('dragging')){card.classList.add('drop-target');moveDraggedPhoto(card,event);}});
    card.addEventListener('dragleave',()=>card.classList.remove('drop-target'));
    card.addEventListener('drop',event=>{if(!$('photosBox').querySelector('.scenic-photo-item.dragging'))return;event.preventDefault();card.classList.remove('drop-target');moveDraggedPhoto(card,event);});
  }
  function addPhotoCard(photo) {
    const card=document.createElement('div');card.className='scenic-photo scenic-photo-item';const img=document.createElement('img');img.draggable=false;img.src=photoURL(photo.url);img.alt=photo.alt_text||photo.caption||'景点图片';
    const source=document.createElement('div');source.className='scenic-photo-source';source.textContent='来源：'+(photo.provider==='amap'?'高德':photo.provider==='google'?'谷歌':'人工');
    const caption=document.createElement('input');caption.className='form-control';caption.placeholder='图片说明';caption.value=photo.caption||'';
    const controls=document.createElement('div');controls.className='d-flex gap-2 mt-2';const remove=document.createElement('button');remove.type='button';remove.className='btn btn-sm btn-outline-danger ms-auto';remove.textContent='移除';remove.addEventListener('click',()=>card.remove());controls.appendChild(remove);
    card.dataset.photoId=photo.id;card.dataset.url=photo.url;card.dataset.altText=photo.alt_text||'';card.dataset.attribution=photo.attribution||'';card.dataset.provider=photo.provider||'';card.dataset.sourceUrl=photo.source_url||'';card.dataset.attributionUrl=photo.attribution_url||'';
    card.append(source,img,caption,controls);$('photosBox').appendChild(card);bindPhotoDragEvents(card,true);
  }
  function addUploadPhotoCard(){
    const card=document.createElement('div');card.className='scenic-photo scenic-photo-add-card';
    const source=document.createElement('div');source.className='scenic-photo-source';source.textContent='来源：人工';
    const picker=document.createElement('button');picker.type='button';picker.className='scenic-photo-picker';picker.setAttribute('aria-label','选择景点图片');picker.innerHTML='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg><span>选择图片</span>';
    const preview=document.createElement('img');preview.hidden=true;picker.appendChild(preview);picker.addEventListener('click',()=>{if(!state.newSpot)$('photoFile').click();else toast('请先保存景点，再上传图片','warning');});
    const caption=document.createElement('input');caption.className='form-control scenic-photo-upload-caption';caption.placeholder='图片说明';
    const controls=document.createElement('div');controls.className='scenic-photo-upload-controls';const upload=document.createElement('button');upload.type='button';upload.className='btn btn-sm btn-outline-primary';upload.textContent='上传';upload.disabled=true;upload.addEventListener('click',uploadPhoto);controls.appendChild(upload);
    card.append(source,picker,caption,controls);$('photosBox').appendChild(card);bindPhotoDragEvents(card,false);
  }
  function renderPhotoCards(photos){$('photosBox').replaceChildren();(photos||[]).forEach(addPhotoCard);addUploadPhotoCard();}
  function updateUploadPhotoPreview(){const card=$('photosBox').querySelector('.scenic-photo-add-card');if(!card)return;const picker=card.querySelector('.scenic-photo-picker'),preview=picker.querySelector('img'),icon=picker.querySelector('svg'),label=picker.querySelector('span'),upload=card.querySelector('button.btn-outline-primary');if(state.photoPreviewURL)URL.revokeObjectURL(state.photoPreviewURL);state.photoPreviewURL=null;const file=$('photoFile').files[0];if(file){state.photoPreviewURL=URL.createObjectURL(file);preview.src=state.photoPreviewURL;preview.hidden=false;icon.hidden=true;label.hidden=true;}else{preview.removeAttribute('src');preview.hidden=true;icon.hidden=false;label.hidden=false;}upload.disabled=!file||!state.current||!state.current.id;}
  function currentPayload() {
    const base=state.current||{};const photos=Array.from($('photosBox').querySelectorAll('.scenic-photo-item')).map(card=>{const caption=card.querySelector('input');return{id:card.dataset.photoId,url:card.dataset.url,caption:caption.value.trim(),alt_text:card.dataset.altText||undefined,attribution:card.dataset.attribution||undefined,attribution_url:card.dataset.attributionUrl||undefined,source_url:card.dataset.sourceUrl||undefined,provider:card.dataset.provider||undefined};});
    return Object.assign({},base,state.pendingCoordinates||{},{name:$('nameInput').value.trim(),category:$('categoryInput').value.trim(),address:$('addressInput').value.trim(),description:$('descriptionInput').value.trim(),photos});
  }
  async function save() {
    try{const body=currentPayload();let spot;if(state.newSpot){spot=await api('/scenic-spots',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});}else{spot=await api('/scenic-spots/'+encodeURIComponent(state.current.id),{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({expected_version:state.current.version,spot:body})});}
      state.newSpot=false;fillForm(spot);$('saveHint').hidden=false;window.clearTimeout(state.saveHintTimer);state.saveHintTimer=window.setTimeout(()=>$('saveHint').hidden=true,2400);await Promise.all([loadList(),loadStats()]);
    }catch(error){toast(error.message,'danger');}
  }
  async function uploadPhoto() {
    if(!state.current)return;const file=$('photoFile').files[0];if(!file){toast('先选择 JPG 或 PNG 图片','danger');return;}const form=new FormData();form.append('file',file);form.append('caption',$('photosBox').querySelector('.scenic-photo-upload-caption').value.trim());
    try{const data=await api('/scenic-spots/'+encodeURIComponent(state.current.id)+'/photos',{method:'POST',body:form});state.current=data.spot;$('detailMeta').textContent='内部 ID：'+state.current.id;clearPhotoPreview();renderPhotoCards(state.current.photos||[]);toast('图片已添加','success');}
    catch(error){toast(error.message,'danger');}
  }
  async function copySpotId() {
    const id=state.current&&state.current.id;if(!id)return;
    try{if(navigator.clipboard&&navigator.clipboard.writeText){await navigator.clipboard.writeText(id);}else{const input=document.createElement('textarea');input.value=id;input.style.position='fixed';input.style.opacity='0';document.body.appendChild(input);input.select();const copied=document.execCommand('copy');input.remove();if(!copied)throw new Error('浏览器不支持复制');}toast('ID 复制成功','success');}
    catch(error){toast('复制失败，请手动复制：'+id,'danger');}
  }

  $('searchBtn').addEventListener('click',loadList);$('searchInput').addEventListener('keydown',event=>{if(event.key==='Enter')loadList();});$('statusFilter').addEventListener('change',renderList);$('cityFilter').addEventListener('change',renderList);$('refreshStatsBtn').addEventListener('click',loadStats);
  $('newBtn').addEventListener('click',()=>{state.newSpot=true;fillForm({status:'draft',manual_edited:true});});$('closeDetailBtn').addEventListener('click',()=>$('detailModal').close());$('detailModal').addEventListener('click',event=>{if(event.target===$('detailModal'))$('detailModal').close();});
  $('saveBtn').addEventListener('click',save);$('photoFile').addEventListener('change',updateUploadPhotoPreview);$('copySpotIdBtn').addEventListener('click',copySpotId);$('chooseMapLocationBtn').addEventListener('click',()=>{if(state.map)setMapSelectionMode(!state.selectingMapLocation);});$('resetMapLocationBtn').addEventListener('click',resetMapLocation);
  loadList();loadStats();syncProviderContent();
})();
