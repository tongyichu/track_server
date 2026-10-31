(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const state = { current: null, list: [], listCursor: "", listTotal: 0, listHasMore: false, newSpot: false, saving: false, providerSyncing: false, pendingCoordinates: null, map: null, mapMarker: null, mapOriginalPosition: null, selectingMapLocation: false, mapSDKPromise: null, amapPluginPromises: {}, regions: null, districtSearch: null, placeSearch: null, poiSearchPoint: null, poiDraft: null, photoPreviewURL: null, relatedSpot: null, relatedCursor: '' };
  let baselineFormSnapshot = null;

  async function api(path, options) {
    const response = await fetch('/admin/api' + path, Object.assign({ credentials: 'same-origin' }, options || {}));
    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.code && payload.code !== 0) throw new Error(payload.error || ('请求失败：' + response.status));
    return payload.data;
  }
  function toast(message, tone) { if (window.AdminUI) window.AdminUI.alert(message, tone); else window.alert(message); }
  function showDescriptionQueryProgress(hint) { hint.replaceChildren(document.createTextNode('正在联网查询景点资料并整理简介'));const dots=document.createElement('span');dots.className='scenic-query-dots';dots.setAttribute('aria-hidden','true');for(let i=0;i<3;i++){const dot=document.createElement('span');dot.textContent='.';dots.appendChild(dot);}hint.appendChild(dots); }
  function textCell(value) { const node=document.createElement('div');node.textContent=value||'—';return node; }
  function isManual(spot) { if(spot.manual_edited)return true;if((spot.sources||[]).some(source=>source&&source.status==='active'&&source.provider_id))return false;return spot.provider_data_status!=='synced'&&!!(spot.name||spot.category||spot.address||spot.description||(spot.photos||[]).length); }
  function modeLabel(spot) { return isManual(spot) ? '人工' : '自动'; }
  function setModeBadge(spot) { const badge=$('detailMode');badge.textContent=modeLabel(spot);badge.className='scenic-mode '+(isManual(spot)?'manual':'automatic'); }
  function photoURL(url) { return (url||'').startsWith('/api/v1/static/')?url.replace('/api/v1/static/','/admin/api/static/'):url; }
  function spotCities(spot) { const cities=spot.related_cities&&spot.related_cities.length?spot.related_cities:[spot.city_name].filter(Boolean);return [...new Set(cities)]; }

  let statsRequest = 0, statsLoaded = false, listRequest = 0, statisticsDays = 30, loadedDays = 30;
  function filterMissing(key) {
    $('missingFilter').value = key;
    $('searchInput').value = '';
    $('statusFilter').value = '';
    $('cityFilter').value = '';
    loadList();
    document.querySelector('.scenic-filter-panel').scrollIntoView({ behavior: 'smooth', block: 'start' });
    $('missingFilter').focus({ preventScroll: true });
  }
  async function loadStats() {
    const request = ++statsRequest, button = $('refreshStatsBtn');
    button.disabled = true;
    button.textContent = '刷新中…';
    window.AdminScenicSpotCharts.setLoading(true);
    $('statsError').hidden = true;
    try {
      const data = await api('/scenic-spots/statistics?days=' + statisticsDays, { cache: 'no-store' });
      if (request !== statsRequest) return;
      const total = Number(data.total_spots || 0), incomplete = Number(data.incomplete_spots || 0);
      $('statsSpotTotal').textContent = total.toLocaleString();
      $('statsCityTotal').textContent = Number(data.covered_cities || 0).toLocaleString();
      $('statsLinkedTracks').textContent = Number(data.linked_tracks || 0).toLocaleString();
      $('statsIncomplete').textContent = incomplete.toLocaleString();
      $('statsIncompleteNote').textContent = '占 ' + (total ? (incomplete * 100 / total).toFixed(1) : '0.0') + '% · 点击查看';
      const date = new Date(data.as_of);
      $('statsUpdated').textContent = '当前有效景点目录' + (Number.isNaN(date.getTime()) ? '' : ' · 更新于 ' + date.toLocaleTimeString('zh-CN', { hour12: false }));
      loadedDays = data.trend.days;
      await window.AdminScenicSpotCharts.render(data, spotID => loadSpot(spotID));
      statsLoaded = true;
    } catch (error) {
      if (request !== statsRequest) return;
      $('statsError').textContent = (statsLoaded ? '刷新失败，仍显示上次数据：' : '统计加载失败：') + error.message;
      $('statsError').hidden = false;
      statisticsDays = loadedDays;
      updateTrendButtons();
      if (!statsLoaded) {
        window.AdminScenicSpotCharts.showError('统计暂不可用，请刷新重试。');
      }
    } finally {
      if (request === statsRequest) { button.disabled = false; button.textContent = '刷新数据'; window.AdminScenicSpotCharts.setLoading(false); }
    }
  }

  function updateTrendButtons() {
    document.querySelectorAll('[data-scenic-days]').forEach(button => {
      const active = Number(button.dataset.scenicDays) === statisticsDays;
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', String(active));
    });
  }
  document.querySelectorAll('[data-scenic-days]').forEach(button => {
    button.addEventListener('click', () => {
      const days = Number(button.dataset.scenicDays);
      if (days === statisticsDays) return;
      statisticsDays = days;
      updateTrendButtons();
      loadStats();
    });
  });

  function renderList() {
    const mode=$('statusFilter').value,city=$('cityFilter').value,query=$('searchInput').value.trim().toLocaleLowerCase();
    const spots=state.list.filter(spot=>(!mode||(mode==='manual')===isManual(spot))&&(!city||spotCities(spot).includes(city))&&(!query||(spot.name||'').toLocaleLowerCase().includes(query)));
    $('spotCount').textContent='当前显示 '+spots.length+' 处'+(state.listHasMore?' · 已加载 '+state.list.length+' / '+state.listTotal+' 处':'');const host=$('spotRows');host.replaceChildren();
    if(!spots.length){const empty=document.createElement('tr');empty.innerHTML='<td colspan="6" class="p-3 scenic-hint">没有匹配的景点</td>';host.appendChild(empty);return;}
    spots.forEach(spot=>{const row=document.createElement('tr');row.dataset.spotId=spot.id;
      const cityCell=document.createElement('td');cityCell.className='city-cell';cityCell.textContent=spotCities(spot).join('、')||'—';
      const nameCell=document.createElement('td');nameCell.className='name-cell';nameCell.textContent=spot.name||'待补充名称';
      const categoryCell=document.createElement('td');categoryCell.className='category-cell';categoryCell.textContent=spot.category||'—';categoryCell.title=spot.category||'';
      const modeCell=document.createElement('td');const badge=document.createElement('span');badge.className='scenic-mode '+(isManual(spot)?'manual':'automatic');badge.textContent=modeLabel(spot);modeCell.appendChild(badge);
      const imageCell=document.createElement('td');const images=document.createElement('div');images.className='scenic-row-photos';(spot.photos||[]).slice(0,3).forEach(photo=>{const img=document.createElement('img');img.loading='lazy';img.src=photoURL(photo.url);img.alt=photo.caption||spot.name||'景点图片';images.appendChild(img);});if(!images.children.length){const hint=document.createElement('span');hint.className='scenic-hint';hint.textContent='暂无图片';images.appendChild(hint);}imageCell.appendChild(images);
      const actionCell=document.createElement('td');const actions=document.createElement('div');actions.className='d-flex gap-2 flex-wrap';const view=document.createElement('button');view.type='button';view.className='btn btn-sm btn-outline-primary';view.textContent='查看详情';view.addEventListener('click',()=>loadSpot(spot.id));const related=document.createElement('button');related.type='button';related.className='btn btn-sm btn-outline-secondary';related.textContent='相关轨迹('+Number(spot.related_track_count||0).toLocaleString()+')';related.dataset.relatedSpotId=spot.id;related.addEventListener('click',()=>openRelatedTracks(spot));actions.append(view,related);actionCell.appendChild(actions);
      row.append(cityCell,nameCell,categoryCell,modeCell,imageCell,actionCell);host.appendChild(row);
    });
  }
  function updateCityOptions() { const select=$('cityFilter'),current=select.value,cities=[...new Set(state.list.flatMap(spotCities))].sort((a,b)=>a.localeCompare(b,'zh-CN'));select.replaceChildren(new Option('全部城市',''));cities.forEach(city=>select.add(new Option(city,city)));if(cities.includes(current))select.value=current; }
  async function loadList(append) {
    append = append === true;
    const request = ++listRequest, button = $('loadMoreSpotsBtn');
    button.disabled = true;
    if (!append) {
      state.list = [];
      state.listCursor = '';
      state.listHasMore = false;
      button.hidden = true;
      $('spotCount').textContent = '加载中…';
      $('spotRows').innerHTML = '<tr><td colspan="6" class="p-3 scenic-hint">正在加载景点…</td></tr>';
    }
    try {
      const params = new URLSearchParams({ q: $('searchInput').value.trim(), limit: '100' });
      if ($('missingFilter').value) params.set('missing', $('missingFilter').value);
      if (append && state.listCursor) params.set('cursor', state.listCursor);
      const data = await api('/scenic-spots?' + params);
      if (request !== listRequest) return;
      state.list = append ? state.list.concat(data.items || []) : data.items || [];
      state.listTotal = Number(data.total || 0);
      state.listCursor = data.next_cursor || '';
      state.listHasMore = Boolean(data.has_more && state.listCursor && state.list.length < state.listTotal);
      updateCityOptions();
      renderList();
    } catch (error) {
      if (request !== listRequest) return;
      toast(error.message, 'danger');
      if (!append) {
        $('spotCount').textContent = '加载失败';
        const cell = document.createElement('td');
        cell.colSpan = 6;
        cell.className = 'p-3 scenic-hint';
        cell.textContent = '加载失败：' + error.message + '。请点击“查询”重试。';
        const row = document.createElement('tr');
        row.appendChild(cell);
        $('spotRows').replaceChildren(row);
      }
    } finally {
      if (request === listRequest) { button.disabled = false; button.hidden = !state.listHasMore; }
    }
  }
  const trackTypeNames={hiking:'徒步',running:'跑步',climbing:'爬山',riding:'骑行',driving:'自驾'};
  const trackTypeColors={hiking:'#16a34a',running:'#2563eb',climbing:'#ea580c',riding:'#9333ea',driving:'#0f766e'};
  let trackCityNames=null;
  function updateRelatedTrackCount(spotID, count) {
    const spot = state.list.find(item => item.id === spotID);
    if (spot) spot.related_track_count = count;
    if (state.relatedSpot && state.relatedSpot.id === spotID) state.relatedSpot.related_track_count = count;
    document.querySelectorAll('[data-related-spot-id]').forEach(button => {
      if (button.dataset.relatedSpotId === spotID) button.textContent = '相关轨迹(' + count.toLocaleString() + ')';
    });
  }
  async function openRelatedTracks(spot){state.relatedSpot=spot;state.relatedCursor='';$('relatedTracksTitle').textContent=(spot.name||'待补充名称')+' · 相关轨迹';$('relatedTracksList').replaceChildren();$('relatedTracksHint').textContent='加载中…';$('loadMoreRelatedTracksBtn').hidden=true;$('relatedTracksModal').showModal();if(!trackCityNames){try{const cities=await api('/cities');trackCityNames=Object.fromEntries((cities||[]).map(city=>[city.code,city.name]));}catch(_){trackCityNames={};}}await loadRelatedTracks(false);}
  async function loadRelatedTracks(append){if(!state.relatedSpot)return;const spotID=state.relatedSpot.id;const button=$('loadMoreRelatedTracksBtn');button.disabled=true;try{const params=new URLSearchParams({limit:'50'});if(state.relatedCursor)params.set('cursor',state.relatedCursor);const data=await api('/scenic-spots/'+encodeURIComponent(spotID)+'/tracks?'+params);if(!state.relatedSpot||state.relatedSpot.id!==spotID)return;const items=data.items||[],host=$('relatedTracksList');if(!append)host.replaceChildren();items.forEach(track=>{const row=document.createElement('tr');row.dataset.trackId=track.track_id;const avatarCell=document.createElement('td');avatarCell.className='track-user-cell';const avatar=document.createElement('span');avatar.className='track-user';if(track.avatar_url){const image=document.createElement('img');image.className='track-avatar';image.loading='lazy';image.src=photoURL(track.avatar_url);image.alt='';avatar.appendChild(image);}else{const fallback=document.createElement('span');fallback.className='track-avatar-fallback';fallback.textContent=(track.user_name||'用').slice(0,1);avatar.appendChild(fallback);}const userName=document.createElement('span');userName.textContent=track.user_name||'未知用户';avatar.appendChild(userName);avatarCell.appendChild(avatar);const imageCell=document.createElement('td');imageCell.className='track-preview';const imageURL=photoURL(track.preview_images&&track.preview_images.landscape_day_url);if(imageURL){const image=document.createElement('img');image.loading='lazy';image.src=imageURL;image.alt='轨迹图';imageCell.appendChild(image);}else{imageCell.textContent='无';}const city=trackCityNames&&trackCityNames[track.city_code]||'未知城市';const typeColor=trackTypeColors[track.track_type]||'#64748b';const typeCell=document.createElement('td');const typeBadge=document.createElement('span');typeBadge.className='track-type-badge';typeBadge.style.setProperty('--track-type-color',typeColor);const typeIcon=document.createElement('span');typeIcon.textContent='●';typeBadge.append(typeIcon,document.createTextNode(trackTypeNames[track.track_type]||track.track_type||'未知'));typeCell.appendChild(typeBadge);const columns=[track.id,city,track.title,typeCell,avatarCell,imageCell,track.status===1?'正常':track.status===2?'私密':track.status===0?'已删除':track.status];columns.forEach(value=>{if(value instanceof HTMLElement){row.appendChild(value);return;}const cell=document.createElement('td');cell.textContent=value===undefined||value===null||value===''?'—':String(value);row.appendChild(cell);});const action=document.createElement('td');const edit=document.createElement('button');edit.type='button';edit.className='btn btn-sm btn-outline-primary';edit.textContent='编辑';edit.addEventListener('click',event=>{event.stopPropagation();openTrackEditor(track.track_id);});action.appendChild(edit);row.appendChild(action);row.addEventListener('click',()=>openTrackEditor(track.track_id));host.appendChild(row);});if(!items.length&&!append){const empty=document.createElement('tr');empty.innerHTML='<td colspan="8" class="p-3 scenic-hint">该景点暂未关联轨迹</td>';host.appendChild(empty);}state.relatedCursor=data.next_cursor||'';const total=Number(data.total||0);$('relatedTracksHint').textContent='共 '+total.toLocaleString()+' 条 · 已加载 '+host.querySelectorAll('tr[data-track-id]').length+' 条';updateRelatedTrackCount(spotID,total);button.hidden=!data.has_more||!state.relatedCursor;}catch(error){$('relatedTracksHint').textContent='加载失败：'+error.message;}finally{button.disabled=false;}}
  function formatDate(value){if(!value)return'';const date=new Date(value);return Number.isNaN(date.getTime())?String(value):date.toLocaleString('zh-CN',{hour12:false});}
  function openTrackEditor(trackID){const modal=$('relatedTracksModal');modal.close();window.AdminTrackEditor.open(trackID,{onSaved:()=>loadRelatedTracks(false),onDeleted:()=>loadRelatedTracks(false)}).catch(error=>toast('读取轨迹详情失败：'+error.message,'danger')).finally(()=>window.setTimeout(()=>{if(!modal.open)modal.showModal();},350));}

  async function syncProviderContent() {
    if(state.providerSyncing)return;state.providerSyncing=true;const progress=$('providerSyncProgress');progress.hidden=false;
    let cursor='',scanned=0,synced=0,coordinates=0,noSource=0,failed=0,errorMessage='';
    try{while(true){try{const params=new URLSearchParams({limit:'5'});if(cursor)params.set('cursor',cursor);const result=await api('/scenic-spots/provider-content/backfill?'+params,{method:'POST'});scanned+=result.scanned||0;synced+=result.synced||0;coordinates+=result.coordinates||0;noSource+=result.no_source||0;failed+=result.failed||0;progress.textContent='供应商资料同步：已扫描 '+scanned+' 处，新增资料 '+synced+' 处，补齐坐标 '+coordinates+' 处，缺少来源 '+noSource+' 处，失败 '+failed+' 处';if(!result.has_more)break;cursor=result.next_cursor||'';if(!cursor)break;}catch(error){errorMessage=error.message;break;}}await Promise.all([loadList(),loadStats()]);progress.textContent='供应商资料同步完成：新增资料 '+synced+' 处，补齐坐标 '+coordinates+' 处，缺少来源 '+noSource+' 处，失败 '+failed+' 处。'+(errorMessage?'同步中断：'+errorMessage:'');}
    finally{state.providerSyncing=false;}
  }

  function fillForm(spot) {
    baselineFormSnapshot=null;
    state.current=spot;$('detailModal').showModal();$('detailTitle').textContent=state.newSpot?'新建景点':(spot.name||'待补充名称');
    $('aiDescriptionCandidate').hidden=true;$('aiDescriptionCandidateText').textContent='';
    $('detailMeta').textContent=spot.id?'内部 ID：'+spot.id:'保存后生成内部景点 ID';setModeBadge(spot);
    $('nameInput').value=spot.name||'';$('categoryInput').value=spot.category||'';$('addressInput').value=spot.address||'';$('descriptionInput').value=spot.description||'';
    const cities=spotCities(spot);$('cityInfo').textContent=cities.length?cities.join('、'):'暂无';
    state.pendingCoordinates=null;state.selectingMapLocation=false;const lat=Number(spot.latitude),lng=Number(spot.longitude),hasCoordinates=Number.isFinite(lat)&&Number.isFinite(lng)&&lat!==0&&lng!==0;$('coordinateInfo').hidden=!hasCoordinates;$('coordinateInfo').textContent=hasCoordinates?'坐标（'+(spot.coordinate_system||'未知坐标系')+'）：'+lat.toFixed(6)+'，'+lng.toFixed(6):'';$('coordinateMap').hidden=false;$('coordinateMapError').hidden=true;$('coordinateMapHelp').hidden=false;$('coordinateMapActions').hidden=false;$('chooseMapLocationBtn').disabled=false;$('chooseMapLocationBtn').textContent=hasCoordinates?'重新选点':'选择位置';$('resetMapLocationBtn').disabled=true;$('coordinateMapActionHint').textContent='地图点击仅用于浏览；选点请先点击“'+$('chooseMapLocationBtn').textContent+'”。';$('coordinateMapImage').hidden=!hasCoordinates||!spot.id;
    $('newCityFields').hidden=!state.newSpot;$('poiCandidatePanel').hidden=true;$('poiDraftPanel').hidden=true;$('providerPhotoPanel').hidden=true;$('providerPhotoCandidates').replaceChildren();state.poiDraft=null;
    if(state.newSpot){populateRegionSelectors(spot.city_code||'');}
    if(hasCoordinates&&spot.id){$('coordinateMapImage').src='/admin/api/scenic-spots/'+encodeURIComponent(spot.id)+'/map?v='+encodeURIComponent(spot.version||'');}else{$('coordinateMapImage').removeAttribute('src');}$('copySpotIdBtn').disabled=!spot.id;
    initializeScenicMap(spot);
    $('providerNameHint').textContent='';$('providerCategoryHint').textContent='';$('providerAddressHint').textContent='';$('providerDescriptionHint').textContent='';clearPhotoPreview();renderPhotoCards(spot.photos||[]);captureFormBaseline();
  }
  function toGCJ02(lat,lng,system){if(system!=='WGS84')return{lat,lng};const inside=lng>=72.004&&lng<=137.8347&&lat>=0.8293&&lat<=55.8271;if(!inside)return{lat,lng};let x=lng-105,y=lat-35,a=-100+2*x+3*y+.2*y*y+.1*x*y+.2*Math.sqrt(Math.abs(x));a+=(20*Math.sin(6*x*Math.PI)+20*Math.sin(2*x*Math.PI))*2/3;a+=(20*Math.sin(y*Math.PI)+40*Math.sin(y/3*Math.PI))*2/3;a+=(160*Math.sin(y/12*Math.PI)+320*Math.sin(y*Math.PI/30))*2/3;let b=300+x+2*y+.1*x*x+.1*x*y+.1*Math.sqrt(Math.abs(x));b+=(20*Math.sin(6*x*Math.PI)+20*Math.sin(2*x*Math.PI))*2/3;b+=(20*Math.sin(x*Math.PI)+40*Math.sin(x/3*Math.PI))*2/3;b+=(150*Math.sin(x/12*Math.PI)+300*Math.sin(x/30*Math.PI))*2/3;const rad=lat/180*Math.PI,magic=1-.00669342162296594323*Math.sin(rad)**2,root=Math.sqrt(magic);return{lat:lat+a*180/((6378245*(1-.00669342162296594323))/(magic*root)*Math.PI),lng:lng+b*180/(6378245/root*Math.cos(rad)*Math.PI)};}
  function loadAMapSDK(config){if(window.AMap)return Promise.resolve(window.AMap);if(state.mapSDKPromise)return state.mapSDKPromise;state.mapSDKPromise=new Promise((resolve,reject)=>{window._AMapSecurityConfig={serviceHost:window.location.origin+config.service_host};const script=document.createElement('script');script.src='https://webapi.amap.com/maps?v=2.0&key='+encodeURIComponent(config.key)+'&plugin=AMap.Scale,AMap.ToolBar';script.onload=()=>window.AMap?resolve(window.AMap):reject(new Error('高德地图 SDK 未就绪'));script.onerror=()=>reject(new Error('高德地图 SDK 加载失败'));document.head.appendChild(script);});return state.mapSDKPromise;}
  function loadAMapPlugin(name){if(window.AMap&&window.AMap[name])return Promise.resolve(window.AMap[name]);if(state.amapPluginPromises[name])return state.amapPluginPromises[name];state.amapPluginPromises[name]=new Promise((resolve,reject)=>{if(!window.AMap){reject(new Error('高德地图未就绪'));return;}window.AMap.plugin(['AMap.'+name],()=>window.AMap[name]?resolve(window.AMap[name]):reject(new Error('高德地图插件不可用：'+name)));});return state.amapPluginPromises[name];}
  function setMapSelectionMode(enabled){state.selectingMapLocation=!!enabled;$('coordinateMapCanvas').classList.toggle('selecting-location',state.selectingMapLocation);const hasSavedPosition=!!state.mapOriginalPosition?.hasCoordinates;$('chooseMapLocationBtn').textContent=state.selectingMapLocation?'取消选点':(hasSavedPosition?'重新选点':'选择位置');$('coordinateMapActionHint').textContent=state.selectingMapLocation?(state.newSpot?'点击景点附近位置查找 POI，再从备选资料中选择。':'请在地图上点击新位置；拖动地图可先浏览到目标区域。'):'地图点击仅用于浏览；选点请先点击“'+$('chooseMapLocationBtn').textContent+'”。';}
  function updateMapMarker(lat,lng){if(!state.mapMarker){state.mapMarker=new window.AMap.Marker({position:[lng,lat],draggable:false});state.map.add(state.mapMarker);}else{state.mapMarker.setPosition([lng,lat]);}}
  async function loadRegions(){if(state.regions)return state.regions;const items=await api('/cities');state.regions=items||[];return state.regions;}
  function populateCityOptions(provinceCode,selectedCode){const cities=(state.regions||[]).filter(item=>item.level==='city'&&item.province_code===provinceCode);const select=$('newCitySelect');select.replaceChildren(new Option('选择城市',''));cities.forEach(item=>select.add(new Option(item.name,item.code)));select.disabled=!provinceCode;if(selectedCode&&cities.some(item=>item.code===selectedCode))select.value=selectedCode;}
  function populateRegionSelectors(cityCode){loadRegions().then(items=>{const provinces=items.filter(item=>item.level==='province');const city=items.find(item=>item.code===cityCode&&item.level==='city');const provinceCode=city?city.province_code:(provinces.some(item=>item.code===cityCode)?cityCode:'');const provinceSelect=$('newProvinceSelect');provinceSelect.replaceChildren(new Option('选择省份',''));provinces.forEach(item=>provinceSelect.add(new Option(item.name,item.code)));provinceSelect.value=provinceCode;populateCityOptions(provinceCode,city?city.code:'');const region=items.find(item=>item.code===(cityCode||provinceCode));if(region){updateCurrentCity(region);focusRegion(region.code);}}).catch(error=>toast('城市选项加载失败：'+error.message,'danger'));}
  function selectedRegion(){const province=$('newProvinceSelect').value,city=$('newCitySelect').value,items=state.regions||[];return items.find(item=>item.code===(city||province))||null;}
  function updateCurrentCity(region){if(!state.current)return;state.current.city_code=region?region.code:'';state.current.city_name=region?region.name:'';$('cityInfo').textContent=region?region.name:'暂无';updateSaveButton();}
  function focusRegion(code){if(!state.map||!code)return;loadAMapPlugin('DistrictSearch').then(DistrictSearch=>{const search=state.districtSearch||(state.districtSearch=new DistrictSearch({subdistrict:0,extensions:'base'}));search.search(code,(status,result)=>{if(status!=='complete'||!result.districtList||!result.districtList.length)return;const center=result.districtList[0].center;if(center&&state.map){state.map.setZoom(code.endsWith('0000')?7:11);state.map.setCenter([center.lng,center.lat]);}});}).catch(error=>toast('无法定位行政区：'+error.message,'warning'));}
  async function chooseRegionFromAddress(component){const items=await loadRegions();const clean=value=>(value||'').replace(/(省|市|自治区|壮族自治区|回族自治区|维吾尔自治区|特别行政区)$/,'');const provinceName=clean(component.province||'');const cityName=clean(typeof component.city==='string'?component.city:'')||clean(component.district||'');const province=items.filter(item=>item.level==='province').find(item=>clean(item.name)===provinceName);if(!province)return;const provinceSelect=$('newProvinceSelect');provinceSelect.value=province.code;populateCityOptions(province.code);const city=items.find(item=>item.level==='city'&&item.province_code===province.code&&clean(item.name)===cityName);if(city)$('newCitySelect').value=city.code;updateCurrentCity(city||province);}
  function providerAddress(value){if(Array.isArray(value))return value.filter(Boolean).map(providerAddress).join('');if(typeof value==='string')return value;if(value&&typeof value==='object')return providerAddress(value.text||value.content||value.name||'');return '';}
  function poiLocation(poi){const location=poi&&poi.location;if(!location)return null;if(Array.isArray(location))return{lng:Number(location[0]),lat:Number(location[1])};if(typeof location.getLng==='function')return{lng:Number(location.getLng()),lat:Number(location.getLat())};if(typeof location.lng==='number'&&typeof location.lat==='number')return{lng:location.lng,lat:location.lat};if(typeof location==='string'){const parts=location.split(',').map(Number);if(parts.length===2)return{lng:parts[0],lat:parts[1]};}return null;}
  function distanceLabel(poi){if(Number.isFinite(Number(poi.distance)))return Number(poi.distance)<1000?Math.round(Number(poi.distance))+' 米':(Number(poi.distance)/1000).toFixed(1)+' 公里';const location=poiLocation(poi),point=state.poiSearchPoint;if(!location||!point)return'';const rad=value=>value*Math.PI/180,dLat=rad(location.lat-point.lat),dLng=rad(location.lng-point.lng),a=Math.sin(dLat/2)**2+Math.cos(rad(point.lat))*Math.cos(rad(location.lat))*Math.sin(dLng/2)**2;const meters=6371000*2*Math.atan2(Math.sqrt(a),Math.sqrt(1-a));return meters<1000?Math.round(meters)+' 米':(meters/1000).toFixed(1)+' 公里';}
  function poiPhotoList(poi){return(poi&&Array.isArray(poi.photos)?poi.photos:[]).map(photo=>{let url=providerAddress(photo.url||photo.photo_url||'');if(/^http:\/\//i.test(url))url=url.replace(/^http:/i,'https:');return{url,title:providerAddress(photo.title||photo.name||'')};}).filter(photo=>/^https:\/\//i.test(photo.url)).slice(0,12);}
  function makePOIDraft(poi,base){const location=poiLocation(poi)||(base&&base.location)||state.poiSearchPoint;const sourceID=String(poi.id||poi.uid||poi.pguid||(base&&base.sourceID)||'').trim();const photos=poiPhotoList(poi);return{sourceID,name:providerAddress(poi.name)||base&&base.name||'',category:providerAddress(poi.type)||base&&base.category||'',description:providerAddress(poi.description||poi.introduction||poi.intro||poi.summary)||base&&base.description||'',address:providerAddress(poi.address)||base&&base.address||'',location,province:providerAddress(poi.pname||poi.province)||base&&base.province||'',city:providerAddress(poi.cityname||poi.city)||base&&base.city||'',district:providerAddress(poi.adname||poi.district)||base&&base.district||'',photos:photos.length?photos:base&&base.photos||[]};}
  function renderPOIDraft(){const draft=state.poiDraft;if(!draft){$('poiDraftPanel').hidden=true;return;}$('poiDraftPanel').hidden=false;$('poiDraftName').textContent=draft.name||'供应商未提供';$('poiDraftCategory').textContent=draft.category||'供应商未提供';$('poiDraftDescription').textContent=draft.description||'高德未提供简介';$('poiDraftAddress').textContent=draft.address||'未提供';$('poiDraftLocation').textContent=(draft.location?Number(draft.location.lat).toFixed(6)+'，'+Number(draft.location.lng).toFixed(6):'无坐标')+(draft.sourceID?' · '+draft.sourceID:' · 无 POI ID');const panel=$('providerPhotoPanel'),host=$('providerPhotoCandidates');host.replaceChildren();panel.hidden=!draft.photos.length;draft.photos.forEach(photo=>{const card=document.createElement('div');card.className='scenic-provider-photo-card';const image=document.createElement('img');image.loading='lazy';image.src=photo.url;image.alt=photo.title||draft.name||'高德景点图片';const caption=document.createElement('small');caption.textContent=photo.title||'高德景点图片';card.append(image,caption);host.appendChild(card);});$('applyPoiDraftBtn').textContent=draft.photos.length?'一键代入资料及 '+draft.photos.length+' 张图片':'一键代入资料';}
  function selectNearbyPOI(poi){if(!poi||!state.poiSearchPoint)return;const id=String(poi.id||poi.uid||poi.pguid||'').trim(),existing=id&&state.list.find(spot=>(spot.sources||[]).some(source=>source.provider==='amap'&&source.provider_id===id&&source.status!=='disabled'));if(existing){toast('这个高德 POI 已在景点目录中：'+(existing.name||existing.id),'warning');return;}state.poiDraft=makePOIDraft(poi);renderPOIDraft();if(!id||!state.placeSearch)return;const selectedID=id;state.placeSearch.getDetails(id,(status,result)=>{if(!state.poiDraft||state.poiDraft.sourceID!==selectedID||status!=='complete')return;const details=result&&result.poiList&&result.poiList.pois&&result.poiList.pois[0];if(!details)return;state.poiDraft=makePOIDraft(details,state.poiDraft);renderPOIDraft();});}
  function selectBaseMapPOI(event){if(!state.newSpot||!event||!event.lnglat)return;const point={lat:Number(event.lnglat.getLat()),lng:Number(event.lnglat.getLng())};if(!Number.isFinite(point.lat)||!Number.isFinite(point.lng))return;const id=String(event.id||'').trim();state.poiSearchPoint=point;state.selectingMapLocation=false;$('coordinateMapCanvas').classList.remove('selecting-location');$('poiCandidatePanel').hidden=false;$('poiCandidateTitle').textContent=id?'正在读取所点击景点资料…':'正在查找附近景点…';$('poiCandidateList').replaceChildren();$('coordinateMapActionHint').textContent='已选中地图景点，资料加载后可在备选区一键代入。';if(!id){searchNearbyPOIs(point.lat,point.lng);return;}loadAMapPlugin('PlaceSearch').then(PlaceSearch=>{if(state.poiSearchPoint!==point)return;if(!state.placeSearch)state.placeSearch=new PlaceSearch({pageSize:10,pageIndex:1,extensions:'all'});state.placeSearch.getDetails(id,(status,result)=>{if(state.poiSearchPoint!==point)return;const poi=status==='complete'&&result&&result.poiList&&result.poiList.pois&&result.poiList.pois[0];if(poi){state.poiDraft=makePOIDraft(poi,{sourceID:id,name:event.name||'',location:point});renderPOIDraft();$('poiCandidateTitle').textContent='地图景点资料已就绪';return;}searchNearbyPOIs(point.lat,point.lng);});}).catch(()=>searchNearbyPOIs(point.lat,point.lng));}
  function applyPOIDraft(){const draft=state.poiDraft;if(!draft)return;if(draft.sourceID){const existing=state.list.find(spot=>(spot.sources||[]).some(source=>source.provider==='amap'&&source.provider_id===draft.sourceID&&source.status!=='disabled'));if(existing){toast('这个高德 POI 已在景点目录中：'+(existing.name||existing.id),'warning');return;}}$('nameInput').value=draft.name||'';$('categoryInput').value=draft.category||'';$('descriptionInput').value=draft.description||'';$('addressInput').value=draft.address||'';if(draft.location){updateMapMarker(draft.location.lat,draft.location.lng);setPendingCoordinates(draft.location.lat,draft.location.lng);$('resetMapLocationBtn').disabled=false;}state.current.sources=draft.sourceID?[{provider:'amap',provider_id:draft.sourceID,status:'active'}]:[];chooseRegionFromAddress({province:draft.province,city:draft.city,district:draft.district}).catch(error=>toast('省市识别失败：'+error.message,'warning'));const existingURLs=new Set(Array.from($('photosBox').querySelectorAll('.scenic-photo-item')).map(card=>card.dataset.url));draft.photos.forEach((photo,index)=>{if(existingURLs.has(photo.url))return;const id='amap-photo-'+Date.now().toString(36)+'-'+index+'-'+Math.random().toString(36).slice(2,7);addPhotoCard({id,url:photo.url,caption:photo.title||'',provider:'amap',source_url:'https://www.amap.com/'});existingURLs.add(photo.url);});if(state.placeSearch)state.placeSearch.clear();state.poiSearchPoint=null;state.poiDraft=null;$('poiCandidatePanel').hidden=true;$('coordinateMapActionHint').textContent=draft.sourceID?'已代入高德景点资料并关联 POI ID。':'已代入景点资料。';updateSaveButton();}
  function useClickedMapLocation(){const point=state.poiSearchPoint;if(!point)return;updateMapMarker(point.lat,point.lng);setPendingCoordinates(point.lat,point.lng);state.current.sources=[];state.poiDraft=null;if(state.placeSearch)state.placeSearch.clear();state.poiSearchPoint=null;$('poiCandidatePanel').hidden=true;$('coordinateMapActionHint').textContent='已使用地图坐标；可手动填写景点名称。';reverseGeocode(point.lat,point.lng);}
  function searchNearbyPOIs(lat,lng){const point={lat:Number(lat),lng:Number(lng)};state.poiSearchPoint=point;$('poiCandidatePanel').hidden=false;$('poiCandidateTitle').textContent='正在查找附近高德景点…';$('poiCandidateList').replaceChildren();loadAMapPlugin('PlaceSearch').then(PlaceSearch=>{if(!state.map||state.poiSearchPoint!==point)return;if(state.placeSearch)state.placeSearch.clear();state.placeSearch=new PlaceSearch({map:state.map,pageSize:10,pageIndex:1,extensions:'all',type:'110000',autoFitView:false,showCover:false});state.placeSearch.on('markerClick',event=>selectNearbyPOI(event.data));state.placeSearch.searchNearBy('风景名胜',[lng,lat],2000,(status,result)=>{if(state.poiSearchPoint!==point)return;const pois=status==='complete'&&result&&result.poiList&&result.poiList.pois||[];const host=$('poiCandidateList');host.replaceChildren();if(!pois.length){$('poiCandidateTitle').textContent='附近没有匹配的高德景点';return;}$('poiCandidateTitle').textContent='附近高德景点 · '+pois.length+' 个';pois.forEach(poi=>{const button=document.createElement('button');button.type='button';button.className='scenic-poi-item';const info=document.createElement('span'),name=document.createElement('strong'),detail=document.createElement('small');name.textContent=poi.name||'未命名景点';detail.textContent=[poi.type,providerAddress(poi.address)].filter(Boolean).join(' · ');info.append(name,detail);const distance=document.createElement('span');distance.className='scenic-poi-distance';distance.textContent=distanceLabel(poi);button.append(info,distance);button.addEventListener('click',()=>selectNearbyPOI(poi));host.appendChild(button);});});}).catch(error=>{if(state.poiSearchPoint!==point)return;$('poiCandidateTitle').textContent='附近景点查询失败';toast('高德景点查询失败：'+error.message,'warning');});}
  function reverseGeocode(lat,lng){if(!window.AMap||!state.map)return;loadAMapPlugin('Geocoder').then(Geocoder=>{const geocoder=new Geocoder({});geocoder.getAddress([lng,lat],(status,result)=>{if(status!=='complete'||!result.regeocode)return;const address=result.regeocode.formattedAddress||'';if(address)$('addressInput').value=address;if(state.newSpot)chooseRegionFromAddress(result.regeocode.addressComponent||{}).catch(error=>toast('省市识别失败：'+error.message,'warning'));});}).catch(error=>toast('地址解析暂不可用：'+error.message,'warning'));}
  async function initializeScenicMap(spot){if(state.map){state.map.destroy();state.map=null;state.mapMarker=null;}state.placeSearch=null;state.poiSearchPoint=null;state.poiDraft=null;state.mapOriginalPosition=null;state.selectingMapLocation=false;$('poiCandidatePanel').hidden=true;$('coordinateMapError').hidden=true;$('coordinateMapHelp').hidden=false;try{const config=await api('/scenic-spots/map-config');if(!config.configured){$('coordinateMapError').textContent='缺少高德 Web(JS API) Key 或安全密钥';$('coordinateMapError').hidden=false;$('chooseMapLocationBtn').disabled=true;$('coordinateMapActionHint').textContent='地图服务未配置，暂时无法选点。';return;}const AMap=await loadAMapSDK(config);if(state.current!==spot)return;const lat=Number(spot.latitude),lng=Number(spot.longitude),hasCoordinates=Number.isFinite(lat)&&Number.isFinite(lng)&&lat!==0&&lng!==0;const center=hasCoordinates?toGCJ02(lat,lng,spot.coordinate_system):{lat:35.8617,lng:104.1954};const zoom=hasCoordinates?15:4;state.mapOriginalPosition={latitude:lat,longitude:lng,coordinateSystem:spot.coordinate_system||'',hasCoordinates,mapLat:center.lat,mapLng:center.lng,zoom};window.AMap=AMap;state.map=new AMap.Map('coordinateMapCanvas',{zoom,center:[center.lng,center.lat],resizeEnable:true,scrollWheel:true});state.map.addControl(new AMap.Scale());state.map.addControl(new AMap.ToolBar({position:{right:'10px',top:'10px'}}));if(hasCoordinates){state.mapMarker=new AMap.Marker({position:[center.lng,center.lat],draggable:false});state.map.add(state.mapMarker);}state.map.on('hotspotclick',selectBaseMapPOI);state.map.on('click',event=>{if(!state.selectingMapLocation)return;const position=event.lnglat;updateMapMarker(position.getLat(),position.getLng());$('resetMapLocationBtn').disabled=false;setMapSelectionMode(false);if(state.newSpot)searchNearbyPOIs(position.getLat(),position.getLng());else{setPendingCoordinates(position.getLat(),position.getLng());reverseGeocode(position.getLat(),position.getLng());}});$('chooseMapLocationBtn').disabled=false;setMapSelectionMode(false);if(state.newSpot&&selectedRegion())focusRegion(selectedRegion().code);}catch(error){$('coordinateMapError').textContent=error.message||'地图暂时无法加载';$('coordinateMapError').hidden=false;$('chooseMapLocationBtn').disabled=true;}}
  function setPendingCoordinates(lat,lng){state.pendingCoordinates={latitude:Number(lat),longitude:Number(lng),coordinate_system:'GCJ02'};$('coordinateInfo').hidden=false;$('coordinateInfo').textContent='坐标（GCJ02）：'+state.pendingCoordinates.latitude.toFixed(6)+'，'+state.pendingCoordinates.longitude.toFixed(6)+' · 未保存';updateSaveButton();}
  function resetMapLocation(){if(!state.map||!state.mapOriginalPosition)return;const original=state.mapOriginalPosition;state.pendingCoordinates=null;state.poiDraft=null;setMapSelectionMode(false);if(state.placeSearch)state.placeSearch.clear();state.poiSearchPoint=null;$('poiDraftPanel').hidden=true;$('poiCandidatePanel').hidden=true;if(original.hasCoordinates){updateMapMarker(original.mapLat,original.mapLng);state.map.setZoom(original.zoom);state.map.setCenter([original.mapLng,original.mapLat]);$('coordinateInfo').hidden=false;$('coordinateInfo').textContent='坐标（'+(original.coordinateSystem||'未知坐标系')+'）：'+Number(original.latitude).toFixed(6)+'，'+Number(original.longitude).toFixed(6);}else{if(state.mapMarker){state.map.remove(state.mapMarker);state.mapMarker=null;}state.map.setZoom(original.zoom);state.map.setCenter([original.mapLng,original.mapLat]);$('coordinateInfo').hidden=true;$('coordinateInfo').textContent='';}$('resetMapLocationBtn').disabled=true;updateSaveButton();}
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
    card.addEventListener('dragend',()=>{card.classList.remove('dragging');$('photosBox').querySelectorAll('.drop-target').forEach(item=>item.classList.remove('drop-target'));updateSaveButton();});
    card.addEventListener('dragover',event=>{if(!$('photosBox').querySelector('.scenic-photo-item.dragging'))return;event.preventDefault();if(!card.classList.contains('dragging')){card.classList.add('drop-target');moveDraggedPhoto(card,event);}});
    card.addEventListener('dragleave',()=>card.classList.remove('drop-target'));
    card.addEventListener('drop',event=>{if(!$('photosBox').querySelector('.scenic-photo-item.dragging'))return;event.preventDefault();card.classList.remove('drop-target');moveDraggedPhoto(card,event);});
  }
  function addPhotoCard(photo) {
    const card=document.createElement('div');card.className='scenic-photo scenic-photo-item';const img=document.createElement('img');img.draggable=false;img.src=photoURL(photo.url);img.alt=photo.alt_text||photo.caption||'景点图片';
    const source=document.createElement('div');source.className='scenic-photo-source';source.textContent='来源：'+(photo.provider==='amap'?'高德':photo.provider==='google'?'谷歌':'人工');
    const caption=document.createElement('input');caption.className='form-control';caption.placeholder='图片说明';caption.value=photo.caption||'';
    const controls=document.createElement('div');controls.className='d-flex gap-2 mt-2';const remove=document.createElement('button');remove.type='button';remove.className='btn btn-sm btn-outline-danger ms-auto';remove.textContent='移除';remove.addEventListener('click',()=>{card.remove();renderProviderPhotoCandidates(state.providerPhotoCandidates);updateSaveButton();});controls.appendChild(remove);caption.addEventListener('input',updateSaveButton);
    card.dataset.photoId=photo.id;card.dataset.url=photo.url;card.dataset.altText=photo.alt_text||'';card.dataset.attribution=photo.attribution||'';card.dataset.provider=photo.provider||'';card.dataset.sourceUrl=photo.source_url||'';card.dataset.attributionUrl=photo.attribution_url||'';
    card.append(source,img,caption,controls);const addCard=$('photosBox').querySelector('.scenic-photo-add-card');if(addCard)$('photosBox').insertBefore(card,addCard);else $('photosBox').appendChild(card);bindPhotoDragEvents(card,true);return card;
  }
  function addUploadPhotoCard(){
    const card=document.createElement('div');card.className='scenic-photo scenic-photo-add-card';
    const source=document.createElement('div');source.className='scenic-photo-source';source.textContent='来源：人工';
    const picker=document.createElement('button');picker.type='button';picker.className='scenic-photo-picker';picker.setAttribute('aria-label','选择景点图片');picker.innerHTML='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg><span>选择图片</span>';
    const preview=document.createElement('img');preview.hidden=true;picker.appendChild(preview);picker.addEventListener('click',()=>$('photoFile').click());
    const caption=document.createElement('input');caption.className='form-control scenic-photo-upload-caption';caption.placeholder='图片说明';caption.addEventListener('input',updateSaveButton);
    const controls=document.createElement('div');controls.className='scenic-photo-upload-controls';const upload=document.createElement('button');upload.type='button';upload.className='btn btn-sm btn-outline-primary';upload.textContent=state.newSpot?'保存并上传':'上传';upload.disabled=true;upload.addEventListener('click',()=>state.newSpot?save():uploadPhoto());controls.appendChild(upload);
    card.append(source,picker,caption,controls);$('photosBox').appendChild(card);bindPhotoDragEvents(card,false);
  }
  function renderPhotoCards(photos){$('photosBox').replaceChildren();(photos||[]).forEach(addPhotoCard);addUploadPhotoCard();}
  function updateUploadPhotoPreview(){const card=$('photosBox').querySelector('.scenic-photo-add-card');if(!card)return;const picker=card.querySelector('.scenic-photo-picker'),preview=picker.querySelector('img'),icon=picker.querySelector('svg'),label=picker.querySelector('span'),upload=card.querySelector('button.btn-outline-primary');if(state.photoPreviewURL)URL.revokeObjectURL(state.photoPreviewURL);state.photoPreviewURL=null;const file=$('photoFile').files[0];if(file){state.photoPreviewURL=URL.createObjectURL(file);preview.src=state.photoPreviewURL;preview.hidden=false;icon.hidden=true;label.hidden=true;}else{preview.removeAttribute('src');preview.hidden=true;icon.hidden=false;label.hidden=false;}upload.textContent=state.newSpot?'保存并上传':'上传';upload.disabled=!file;updateSaveButton();}
  function currentPayload() {
    const base=state.current||{};const photos=Array.from($('photosBox').querySelectorAll('.scenic-photo-item')).map(card=>{const caption=card.querySelector('input');return{id:card.dataset.photoId,url:card.dataset.url,caption:caption.value.trim(),alt_text:card.dataset.altText||undefined,attribution:card.dataset.attribution||undefined,attribution_url:card.dataset.attributionUrl||undefined,source_url:card.dataset.sourceUrl||undefined,provider:card.dataset.provider||undefined};});
    return Object.assign({},base,state.pendingCoordinates||{},{name:$('nameInput').value.trim(),category:$('categoryInput').value.trim(),address:$('addressInput').value.trim(),description:$('descriptionInput').value.trim(),photos});
  }
  function formSnapshot(){const payload=currentPayload(),file=state.newSpot?$('photoFile').files[0]:null,uploadCard=$('photosBox').querySelector('.scenic-photo-add-card');return{fields:[payload.name,payload.category,payload.address,payload.description,payload.city_code||'',payload.latitude==null?null:Number(payload.latitude),payload.longitude==null?null:Number(payload.longitude),payload.coordinate_system||''],sources:(payload.sources||[]).map(source=>[source.provider||'',source.provider_id||'',source.status||'']),photos:payload.photos.map(photo=>[photo.id||'',photo.url||'',photo.caption||'',photo.provider||'']),pendingFile:file?[file.name,file.size,file.lastModified,uploadCard?.querySelector('.scenic-photo-upload-caption')?.value.trim()||'']:null};}
  function updateSaveButton(){const button=$('saveBtn');if(!button)return;const changed=baselineFormSnapshot!==null&&JSON.stringify(formSnapshot())!==baselineFormSnapshot;button.disabled=state.saving||!changed;}
  function captureFormBaseline(){baselineFormSnapshot=JSON.stringify(formSnapshot());updateSaveButton();}
  function updateBaselinePhotos(){if(!baselineFormSnapshot)return;const baseline=JSON.parse(baselineFormSnapshot);baseline.photos=formSnapshot().photos;baselineFormSnapshot=JSON.stringify(baseline);updateSaveButton();}
  async function save() {
    if(state.saving||$('saveBtn').disabled)return;
    if(state.newSpot&&state.poiDraft){toast('先点击“一键代入资料”，或按此坐标新建。','warning');return;}
    state.saving=true;updateSaveButton();
    try{const body=currentPayload(),pendingFile=state.newSpot?$('photoFile').files[0]:null,pendingCaption=state.newSpot?($('photosBox').querySelector('.scenic-photo-upload-caption')||{}).value||'':'';let spot;if(state.newSpot){spot=await api('/scenic-spots',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});state.newSpot=false;state.current=spot;$('newCityFields').hidden=true;const uploadButton=$('photosBox').querySelector('.scenic-photo-add-card button.btn-outline-primary');if(uploadButton)uploadButton.textContent='上传';if(pendingFile)spot=await uploadPhotoToSpot(spot,pendingFile,pendingCaption);}else{spot=await api('/scenic-spots/'+encodeURIComponent(state.current.id),{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({expected_version:state.current.version,spot:body})});}
      state.newSpot=false;state.current=spot;$('detailModal').close();await Promise.all([loadList(),loadStats()]);toast('保存成功','success');
    }catch(error){toast(error.message,'danger');}
    finally{state.saving=false;updateSaveButton();}
  }
  async function generateScenicDescription() {
    const spot=currentPayload();if(!spot.name){toast('请先填写景点名称','warning');return;}
    const button=$('generateDescriptionBtn'),hint=$('providerDescriptionHint');$('aiDescriptionCandidate').hidden=true;$('aiDescriptionCandidateText').textContent='';button.setAttribute('aria-disabled','true');showDescriptionQueryProgress(hint);
    try{const result=await api('/scenic-spots/description/generate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(spot)});if(!result.description){hint.textContent='没有查到足够可靠的介绍信息';toast('没有查到足够可靠的景点介绍','warning');return;}$('aiDescriptionCandidateText').textContent=result.description;$('aiDescriptionCandidate').hidden=false;hint.textContent='简介已生成，请确认后代入';}
    catch(error){hint.textContent='查询失败：'+error.message;toast(error.message,'danger');}
    finally{button.removeAttribute('aria-disabled');}
  }
  $('applyAiDescriptionBtn').addEventListener('click',()=>{const candidate=$('aiDescriptionCandidateText').textContent.trim();if(!candidate)return;$('descriptionInput').value=candidate;$('aiDescriptionCandidate').hidden=true;$('providerDescriptionHint').textContent='已代入简介，保存后写入景点资料';updateSaveButton();});
  async function uploadPhotoToSpot(spot,file,caption){const form=new FormData();form.append('file',file);form.append('caption',caption||'');const data=await api('/scenic-spots/'+encodeURIComponent(spot.id)+'/photos',{method:'POST',body:form});return data.spot;}
  async function uploadPhoto() {
    if(!state.current)return;const file=$('photoFile').files[0];if(!file){toast('先选择 JPG 或 PNG 图片','danger');return;}const form=new FormData();form.append('file',file);form.append('caption',$('photosBox').querySelector('.scenic-photo-upload-caption').value.trim());
    try{const data=await api('/scenic-spots/'+encodeURIComponent(state.current.id)+'/photos',{method:'POST',body:form});state.current=data.spot;$('detailMeta').textContent='内部 ID：'+state.current.id;clearPhotoPreview();renderPhotoCards(state.current.photos||[]);updateBaselinePhotos();toast('图片已添加','success');}
    catch(error){toast(error.message,'danger');}
  }
  async function copySpotId() {
    const id=state.current&&state.current.id;if(!id)return;
    try{if(navigator.clipboard&&navigator.clipboard.writeText){await navigator.clipboard.writeText(id);}else{const input=document.createElement('textarea');input.value=id;input.style.position='fixed';input.style.opacity='0';document.body.appendChild(input);input.select();const copied=document.execCommand('copy');input.remove();if(!copied)throw new Error('浏览器不支持复制');}toast('ID 复制成功','success');}
    catch(error){toast('复制失败，请手动复制：'+id,'danger');}
  }

  $('searchBtn').addEventListener('click',()=>loadList());$('missingFilter').addEventListener('change',()=>loadList());$('showIncompleteBtn').addEventListener('click',()=>filterMissing('any'));$('loadMoreSpotsBtn').addEventListener('click',()=>loadList(true));$('searchInput').addEventListener('keydown',event=>{if(event.key==='Enter')loadList();});$('statusFilter').addEventListener('change',renderList);$('cityFilter').addEventListener('change',renderList);$('refreshStatsBtn').addEventListener('click',loadStats);
  $('newProvinceSelect').addEventListener('change',()=>{const code=$('newProvinceSelect').value;populateCityOptions(code);const province=(state.regions||[]).find(item=>item.code===code);updateCurrentCity(province);focusRegion(code);});$('newCitySelect').addEventListener('change',()=>{const region=selectedRegion();updateCurrentCity(region);focusRegion(region&&region.code);});$('useClickedLocationBtn').addEventListener('click',useClickedMapLocation);$('applyPoiDraftBtn').addEventListener('click',applyPOIDraft);
  $('newBtn').addEventListener('click',()=>{state.newSpot=true;fillForm({status:'draft',manual_edited:true});});$('closeDetailBtn').addEventListener('click',()=>$('detailModal').close());$('detailModal').addEventListener('click',event=>{if(event.target===$('detailModal'))$('detailModal').close();});
  $('saveBtn').addEventListener('click',save);$('generateDescriptionBtn').addEventListener('click',event=>{event.preventDefault();if(event.currentTarget.getAttribute('aria-disabled')!=='true')generateScenicDescription();});$('photoFile').addEventListener('change',updateUploadPhotoPreview);$('copySpotIdBtn').addEventListener('click',copySpotId);$('chooseMapLocationBtn').addEventListener('click',()=>{if(state.map)setMapSelectionMode(!state.selectingMapLocation);});$('resetMapLocationBtn').addEventListener('click',resetMapLocation);
  ['nameInput','categoryInput','addressInput','descriptionInput'].forEach(id=>$(id).addEventListener('input',updateSaveButton));
  $('closeRelatedTracksBtn').addEventListener('click',()=>$('relatedTracksModal').close());$('relatedTracksModal').addEventListener('click',event=>{if(event.target===$('relatedTracksModal'))$('relatedTracksModal').close();});$('loadMoreRelatedTracksBtn').addEventListener('click',()=>loadRelatedTracks(true));
  loadList();loadStats();syncProviderContent();
})();
