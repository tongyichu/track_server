(function () {
  const body = document.getElementById('musicList'), error = document.getElementById('musicError');
  const modal = document.getElementById('musicEditor'), form = document.getElementById('musicForm');
  const save = document.getElementById('musicSave'), saveError = document.getElementById('musicSaveError');
  let current = null, controller = null, version = 0, saving = false, saveController = null;
  let disposed = false;
  const deleteConfirm = document.getElementById('musicDeleteConfirm'), actionStatus = document.getElementById('musicActionStatus');
  let items = [], loading = false, arranging = false, needsRefresh = false, dragged = null, layoutController = null;
  const status = document.getElementById('musicStatus');
  let activeMarquee = null, trackPreview = null;

  function updateMarquee() {
    if (!activeMarquee) return;
    const {title, titleText} = activeMarquee;
    const width = titleText.getBoundingClientRect().width;
    title.style.setProperty('--music-title-duration', `${Math.max(3, width / 40)}s`);
    title.classList.toggle('music-title-scrolling', width > title.clientWidth + 1);
  }

  const marqueeObserver = new ResizeObserver(updateMarquee);

  function stopMarquee(audio) {
    if (!activeMarquee || (audio && activeMarquee.audio !== audio)) return;
    marqueeObserver.disconnect();
    activeMarquee.title.classList.remove('music-title-scrolling');
    activeMarquee = null;
  }

  function bindTitleMarquee(audio, title, titleText) {
    audio.addEventListener('playing', () => {
      if (disposed || document.hidden || audio.paused || !audio.isConnected) return;
      stopMarquee();
      activeMarquee = {audio, title, titleText};
      marqueeObserver.observe(title);
      marqueeObserver.observe(titleText);
      updateMarquee();
    });
    for (const event of ['pause', 'ended', 'error', 'emptied', 'waiting']) {
      audio.addEventListener(event, () => stopMarquee(audio));
    }
  }
  function writable() { return AdminAccess.can('music',true) && !loading && !arranging && !saving && !needsRefresh; }
  function controls() {
    if(disposed)return;
    document.getElementById('musicRefresh').disabled=loading||arranging||saving;
    document.getElementById('musicAdd').disabled=!writable();
    body.setAttribute('aria-busy', String(loading||arranging));
    body.querySelectorAll('[data-music-write]').forEach(button=>button.disabled=!writable()||button.hasAttribute('data-music-unavailable'));
    body.querySelectorAll('.music-drag-handle').forEach(handle=>handle.draggable=writable());
    save.disabled=!writable();document.getElementById('musicClose').disabled=saving;
    ['musicDelete','musicUnpublish','musicDeleteConfirmButton','musicDeleteCancel'].forEach(id=>document.getElementById(id).disabled=!writable());
    document.getElementById('musicDelete').hidden=!current;
    document.getElementById('musicUnpublish').hidden=!current||!current.published;
  }
  function text(tag, value, className) { const el = document.createElement(tag); el.textContent = value; if(className) el.className=className; return el; }
  function formatTime(ms, fraction=false) {
    const seconds=Math.floor(ms/1000);
    const time=String(Math.floor(seconds/60)).padStart(2,'0')+':'+String(seconds%60).padStart(2,'0');
    const milliseconds=String(ms%1000).padStart(3,'0').replace(/0+$/,'');
    return fraction&&milliseconds ? time+'.'+milliseconds : time;
  }
  async function request(url, options) { const response=await fetch(url,options); const data=await response.json().catch(()=>({})); if(!response.ok) throw new Error(data.error || '读取失败（'+response.status+'）'); return data.data; }
  const previewBox=document.getElementById('musicPreviewBox'), previewRoot=document.getElementById('musicPreviewPlayer');
  const previewState=document.getElementById('musicPreviewState');
  let preview=null, previewObjectURL=null, previewVersion=0, previewNeedsStart=true, replacingAudio=false;
  const startInput=form.elements.start_seconds;
  const currentAudio=document.getElementById('musicCurrentAudio');
  const currentAudioInfo=document.getElementById('musicCurrentAudioInfo');
  const currentCover=document.getElementById('musicCurrentCover');
  const coverImage=document.getElementById('musicCoverImage');
  const coverField=document.getElementById('musicCoverField');
  const audioFileField=document.getElementById('musicFileField'), audioLinkField=document.getElementById('musicLinkField');
  function startSeconds() { return Number(startInput.value || 0); }
  function seekPreview(player) {
    const start=startSeconds();
    if(!Number.isFinite(start)||start<0||start>=1200){previewState.textContent='请填写有效的起播时间。';return false;}
    if(Number.isFinite(player.duration)&&start>=player.duration){previewState.textContent='起播时间必须小于音乐时长。';return false;}
    if(player.readyState>0){try{player.currentTime=start;previewNeedsStart=false;}catch(_){previewState.textContent='此音频暂不支持定位，请检查直链。';return false;}}
    return true;
  }
  startInput.addEventListener('input',()=>{startInput.setCustomValidity('');previewNeedsStart=true;});
  document.getElementById('musicPreviewStart').onclick=()=>{
    if(!preview)showPreview();
    if(!preview) {
      if(previewBox.hidden){previewBox.hidden=false;previewState.textContent='请先选择音频文件或填写链接。';}
      return;
    }
    previewBox.hidden=false;
    previewNeedsStart=true;if(!seekPreview(preview))return;
    const player=preview, run=previewVersion;previewState.textContent='正在加载音频…';
    player.play().catch(error=>{if(run===previewVersion&&preview===player&&error.name!=='AbortError')previewState.textContent='暂时无法试听，请检查音频是否可播放。';});
  };
  function stopPreview() {
    previewVersion++;
    if(preview){preview.pause();preview.removeAttribute('src');preview.load();preview.remove();preview=null;}
    if(previewObjectURL){URL.revokeObjectURL(previewObjectURL);previewObjectURL=null;}
    previewBox.hidden=true;previewRoot.hidden=true;
  }
  function showPreview() {
    stopPreview();startInput.setCustomValidity('');previewNeedsStart=true;const run=previewVersion;
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
    previewRoot.append(player);
    player.addEventListener('loadedmetadata',()=>{
      if(run!==previewVersion)return;
      const duration=player.duration;
      if(Number.isFinite(duration)&&duration>=1&&duration<=1200&&form.elements.audio_mode.value==='link')form.elements.duration_seconds.value=String(Math.round(duration*1000)/1000);
      if(previewNeedsStart&&!seekPreview(player)){player.pause();return;}
      if(Number.isFinite(duration)&&duration>0)previewState.textContent='时长 '+formatTime(Math.round(duration*1000));
    });
    player.addEventListener('play',()=>{if(run===previewVersion&&previewNeedsStart&&!seekPreview(player))player.pause();});
    player.addEventListener('ended',()=>{if(run===previewVersion){previewNeedsStart=true;}});
    player.addEventListener('playing',()=>{if(run===previewVersion){previewRoot.hidden=false;previewState.textContent='正在试听';trackPreview?.audio.pause();}});
    player.addEventListener('error',()=>{if(run===previewVersion)previewState.textContent='暂时无法播放，请检查直链是否有效或被原站限制。';});
  }
  function audioMode() {
    const link=form.elements.audio_mode.value==='link', writable=AdminAccess.can('music',true);
    const keepCurrent=current&&!replacingAudio&&link===Boolean(current.audio_link);
    currentAudio.hidden=!keepCurrent;
    if(keepCurrent) {
      currentAudioInfo.textContent=[link?'当前音频 · 外链':'当前音频 · MP3',current.duration_ms>0?formatTime(current.duration_ms):'',current.audio_bytes>0?(current.audio_bytes/1048576).toFixed(1)+' MB':''].filter(Boolean).join(' · ');
    }
    audioFileField.hidden=link||keepCurrent;
    audioLinkField.hidden=!link||keepCurrent;
    form.elements.audio.disabled=link||!writable;form.elements.audio.required=!link&&(!current||Boolean(current.audio_link));
    form.elements.audio_link.disabled=!link||!writable;form.elements.audio_link.required=link;
    form.elements.duration_seconds.disabled=!link||!writable;
    showPreview();
  }
  document.getElementById('musicReplaceAudio').onclick=()=>{
    replacingAudio=true;audioMode();
    (form.elements.audio_mode.value==='link'?form.elements.audio_link:form.elements.audio).focus();
  };
  document.getElementById('musicReplaceCover').onclick=()=>{coverField.hidden=false;form.elements.cover.focus();};
  form.elements.audio_mode.addEventListener('change',audioMode);
  form.elements.audio_link.addEventListener('input',()=>{startInput.value='0';form.elements.duration_seconds.value='';showPreview();});
  form.elements.audio.addEventListener('change',()=>{startInput.value='0';form.elements.duration_seconds.value='';showPreview();});
  modal.addEventListener('close',()=>{stopPreview();coverImage.removeAttribute('src');});
  function edit(item) {
    if(!writable())return;current=item;replacingAudio=false;deleteConfirm.hidden=true;actionStatus.textContent='';trackPreview?.audio.pause();form.reset();saveError.hidden=true;
    document.getElementById('musicEditTitle').textContent=item?'编辑音乐':'添加音乐';
    ['name','artist'].forEach(key=>{form.elements[key].value=item?item[key]:'';});
    form.elements.audio_mode.value=item&&item.audio_link?'link':'upload';
    form.elements.audio_link.value=item&&item.audio_link||'';
    form.elements.duration_seconds.value=item&&item.audio_link&&item.duration_ms>0?String(item.duration_ms/1000):'';
    startInput.value=String((item&&item.start_offset_ms||0)/1000);startInput.setCustomValidity('');
    currentCover.hidden=!item?.cover_url;
    coverField.hidden=Boolean(item?.cover_url);
    if(item?.cover_url)coverImage.src=item.cover_url;
    audioMode();controls();
    modal.showModal();
    document.getElementById('musicEditorBody').scrollTop=0;
  }
  function icon(name) {
    const img=document.createElement('img');img.src=AdminAssets.url('/admin/static/vendor/tabler-icons/'+name+'.svg');img.alt='';img.draggable=false;img.dataset.noPreview='';return img;
  }
  function closeTrackPreview() {
    if(!trackPreview)return;
    const {audio,tile,button,panel}=trackPreview;
    trackPreview=null;
    stopMarquee(audio);
    audio.pause();audio.removeAttribute('src');audio.load();
    panel.remove();tile.classList.remove('music-tile-active');
    button.dataset.playing='false';button.setAttribute('aria-expanded','false');
    button.setAttribute('aria-label',button.dataset.playLabel);button.title=button.dataset.playLabel;
  }
  function audition(item,tile,button,title,titleText) {
    if(trackPreview?.tile===tile){trackPreview.toggle();return;}
    closeTrackPreview();
    const audio=document.createElement('audio');
    audio.preload='none';audio.src=item.audio_url;audio.hidden=true;
    const start=(item.start_offset_ms||0)/1000;
    let startPending=true;
    let state='idle';
    const panel=text('div','','music-track-preview');
    const line=text('div','','music-timeline');
    const range=document.createElement('input');range.type='range';range.min='0';range.step='0.01';
    range.setAttribute('aria-label','试听进度 · '+item.name);
    const marker=text('span','','music-start-marker');
    marker.setAttribute('aria-hidden','true');
    const details=text('div','','music-preview-details');
    const clock=text('span','','music-time');
    const message=text('span','','hint');message.setAttribute('role','status');
    line.append(range,marker);details.append(clock,message);panel.append(line,details,audio);
    tile.append(panel);tile.classList.add('music-tile-active');button.setAttribute('aria-expanded','true');
    const active=()=>trackPreview?.audio===audio;
    function setState(next, label) {
      if(!active())return;
      state=next;message.textContent=label;
      const playing=next==='playing'||next==='loading';
      button.dataset.playing=String(playing);
      button.setAttribute('aria-label',(playing?'暂停试听 ':'试听 ')+item.name);
      button.title=playing?'暂停试听':'试听';
    }
    function updateTime() {
      if(!active())return;
      const duration=Number.isFinite(audio.duration)&&audio.duration>0?audio.duration:(item.duration_ms||0)/1000;
      const seconds=startPending?start:audio.currentTime;
      range.max=String(duration||1);range.value=String(Math.min(seconds,duration||1));
      range.disabled=audio.readyState===0||!duration;
      range.setAttribute('aria-valuetext',formatTime(Math.round(seconds*1000)));
      marker.hidden=!duration||!start||start>=duration;
      const ratio=duration?Math.min(1,start/duration):0;
      marker.style.left='calc('+(ratio*100)+'% + '+(6-ratio*12)+'px)';
      range.style.setProperty('--music-progress',(duration?Math.min(100,seconds/duration*100):0)+'%');
      clock.textContent=formatTime(Math.round(seconds*1000))+' / '+(duration?formatTime(Math.round(duration*1000)):'–:––');
    }
    function fail(label) {
      setState('error',label);audio.pause();
    }
    function seekStart() {
      if(!active()||!startPending||audio.readyState===0)return;
      if(Number.isFinite(audio.duration)&&start>=audio.duration){fail('起播点超出时长');return;}
      try {audio.currentTime=start;startPending=false;}
      catch(_){fail('无法跳到起播点');}
    }
    function toggle() {
      if(state==='loading'||state==='playing') {
        setState('paused','已暂停');audio.pause();return;
      }
      if(audio.error){startPending=true;audio.load();}
      if(audio.ended||state==='ended')startPending=true;
      setState('loading','正在加载…');
      seekStart();updateTime();
      if(state==='error')return;
      audio.play().catch(error=>{if(active()&&error.name!=='AbortError')fail('试听失败，点击重试');});
    }
    trackPreview={audio,tile,button,panel,toggle};
    audio.addEventListener('loadedmetadata',()=>{seekStart();updateTime();});
    audio.addEventListener('timeupdate',updateTime);
    audio.addEventListener('waiting',()=>{if(!audio.paused)setState('loading','正在缓冲…');});
    audio.addEventListener('playing',()=>setState('playing','正在试听'));
    audio.addEventListener('pause',()=>{if(state==='playing'||state==='loading')setState('paused','已暂停');});
    audio.addEventListener('ended',()=>setState('ended','试听结束'));
    audio.addEventListener('error',()=>{if(active())fail('试听失败，点击重试');});
    range.addEventListener('input',()=>{audio.currentTime=Number(range.value);updateTime();});
    bindTitleMarquee(audio,title,titleText);updateTime();toggle();
  }
  function trackActions(item,group,index) {
    const wrapper=text('div','','dropdown music-actions');
    wrapper.dataset.adminWrite='music';
    const toggle=text('button','','btn btn-sm btn-outline-secondary music-action-toggle');
    toggle.type='button';toggle.dataset.bsToggle='dropdown';toggle.dataset.musicWrite='';
    toggle.setAttribute('aria-expanded','false');toggle.setAttribute('aria-label','排序和操作 '+item.name);
    toggle.title='曲目操作';toggle.append(icon('dots'));
    toggle.append(text('span','排序','music-touch-action'));
    const menu=text('div','','dropdown-menu dropdown-menu-end');
    const addOption=(label,callback,unavailable=false)=>{
      const option=text('button',label,'dropdown-item');option.type='button';option.dataset.musicWrite='';
      if(unavailable)option.dataset.musicUnavailable='';
      option.addEventListener('click',callback);menu.append(option);
    };
    const move=offset=>{
      if(!writable())return;
      const next=items.slice(),from=next.indexOf(item),to=next.indexOf(group[index+offset]);
      if(from<0||to<0)return;
      [next[from],next[to]]=[next[to],next[from]];
      arrange(next,offset<0?'已上移':'已下移',item.id);
    };
    addOption('编辑',()=>edit(item));
    addOption(item.published?'下架':'上架',()=>{
      if(!writable())return;
      const next=items.filter(track=>track.id!==item.id);
      next.push({...item,published:!item.published});
      arrange(next,item.published?'已下架':'已上架',item.id);
    });
    addOption('上移',()=>move(-1),index===0);
    addOption('下移',()=>move(1),index===group.length-1);
    wrapper.append(toggle,menu);return wrapper;
  }
  function render() {
    if(disposed)return;
    closeTrackPreview();
    body.querySelectorAll('.music-action-toggle').forEach(toggle=>tabler.Dropdown.getInstance(toggle)?.dispose());
    body.replaceChildren();body.classList.remove('is-dragging');
    [true,false].forEach(published=>{
      const section=text('section','','music-section'), heading=text('div','','music-section-heading');
      const group=items.filter(item=>item.published===published);
      heading.append(text('h2',published?'已上架':'未上架'),text('span',String(group.length),'badge bg-secondary-lt'));
      const grid=text('div','','music-grid');grid.dataset.published=String(published);
      grid.setAttribute('aria-label',published?'已上架音乐':'未上架音乐');
      group.forEach((item,index)=>{
        const tile=text('article','','card music-tile');tile.dataset.id=item.id;
        const inner=text('div','','card-body'), head=text('div','','music-tile-head');
        const cover=text('div','','music-cover');
        if(item.cover_url){const img=document.createElement('img');img.src=item.cover_url;img.alt='';img.loading='lazy';img.draggable=false;img.dataset.noPreview='';cover.append(img);}
        else {cover.classList.add('music-cover-placeholder');cover.append(icon('music'));}
        const info=text('div','','music-info');
        const title = text('strong', '', 'music-title');
        const titleTrack = text('span', '', 'music-title-track');
        const titleText = text('span', item.name, 'music-title-text');
        const titleRepeat = text('span', item.name, 'music-title-text music-title-repeat');
        title.title = item.name;
        titleText.dir = titleRepeat.dir = 'auto';
        titleRepeat.setAttribute('aria-hidden', 'true');
        titleTrack.append(titleText, titleRepeat);
        title.append(titleTrack);
        info.append(title, text('small', item.artist));
        const duration=item.duration_ms>0?formatTime(item.duration_ms):'时长未知';
        info.append(text('small',duration+(item.start_offset_ms>0?' · 起播 '+formatTime(item.start_offset_ms,true):''),'music-meta music-time'));
        const play=text('button','','btn btn-sm btn-outline-secondary music-play');
        play.type='button';play.dataset.playLabel='试听 '+item.name;
        play.setAttribute('aria-label',play.dataset.playLabel);play.setAttribute('aria-expanded','false');play.title='试听';
        play.addEventListener('click',()=>audition(item,tile,play,title,titleText));
        head.append(cover,info,play);
        if(AdminAccess.can('music',true)) {
          info.dataset.editable='true';info.addEventListener('click',()=>edit(item));
          const handle=text('span','','music-drag-handle');handle.title='拖动排序';handle.dataset.adminWrite='music';handle.setAttribute('aria-hidden','true');
          head.prepend(handle);head.append(trackActions(item,group,index));
          handle.addEventListener('dragstart',event=>{
            if(!writable()){event.preventDefault();return;}
            dragged=item.id;trackPreview?.audio.pause();
            event.dataTransfer.setData('text/plain',item.id);event.dataTransfer.effectAllowed='move';
            event.dataTransfer.setDragImage(tile,20,20);tile.classList.add('dragging');body.classList.add('is-dragging');
          });
          handle.addEventListener('dragend',()=>{if(dragged){dragged=null;render();}});
        }
        inner.append(head);
        tile.append(inner);grid.append(tile);
      });
      const empty=text('div',published?(AdminAccess.can('music',true)?'拖到这里上架，展示到客户端':'暂无已上架音乐'):'暂无未上架音乐','music-empty');grid.append(empty);
      grid.addEventListener('dragover',event=>{
        if(!dragged||!writable())return;event.preventDefault();event.dataTransfer.dropEffect='move';
        body.querySelectorAll('.music-grid').forEach(el=>el.classList.toggle('over',el===grid));
        const tile=body.querySelector('[data-id="'+CSS.escape(dragged)+'"]'), target=event.target.closest('.music-tile');if(!tile||target===tile)return;
        if(target&&target.parentElement===grid){
          const rect=target.getBoundingClientRect();
          const singleColumn=!getComputedStyle(grid).gridTemplateColumns.includes(' ');
          const before=singleColumn?event.clientY<rect.top+rect.height/2:event.clientX<rect.left+rect.width/2;
          grid.insertBefore(tile,before?target:target.nextElementSibling);
        }
        else grid.insertBefore(tile,empty);
      });
      grid.addEventListener('drop',event=>{
        if(!dragged||!writable())return;event.preventDefault();dragged=null;
        const next=[];body.querySelectorAll('.music-grid').forEach(zone=>zone.querySelectorAll('.music-tile').forEach(tile=>{const item=items.find(v=>v.id===tile.dataset.id);next.push({...item,published:zone.dataset.published==='true'});}));
        arrange(next);
      });
      section.append(heading,grid);body.append(section);
    });controls();
  }
  async function load(keepError=false) {
    if(disposed)return false;
    if(controller)controller.abort();controller=new AbortController();const active=controller, run=++version;loading=true;
    if(!keepError)error.hidden=true;controls();const timeout=setTimeout(()=>active.abort(),12000);
    try {const data=await request('/admin/api/music',{signal:active.signal});if(run!==version)return false;items=data.items;needsRefresh=false;render();return true;}
    catch(e){if(run===version){error.textContent=(keepError?error.textContent+'；':'')+(e.name==='AbortError'?'读取超时，请刷新重试。':e.message);error.hidden=false;}return false;}
    finally{clearTimeout(timeout);if(run===version){loading=false;controls();}}
  }
  async function arrange(next, feedback='已保存顺序', focusId=null) {
    if(!writable())return;
    next=[...next.filter(v=>v.published),...next.filter(v=>!v.published)];
    if(next.every((v,i)=>v.id===items[i].id&&v.published===items[i].published)){render();return;}
    arranging=true;status.textContent='正在保存…';error.hidden=true;controls();
    layoutController=new AbortController();const timeout=setTimeout(()=>layoutController.abort(),25000);
    try {const data=await request('/admin/api/music/layout',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({items:next.map(v=>({id:v.id,revision:v.revision,published:v.published}))}),signal:layoutController.signal});items=data.items;render();status.textContent=feedback;}
    catch(e){needsRefresh=true;error.textContent=e.name==='AbortError'?'保存超时，正在重新读取实际结果。':e.message+'，正在重新读取实际结果。';error.hidden=false;status.textContent='';await load(true);}
    finally{
      clearTimeout(timeout);layoutController=null;arranging=false;controls();
      if(focusId&&!disposed)body.querySelector('[data-id="'+CSS.escape(focusId)+'"] .music-action-toggle')?.focus();
    }
  }
  document.getElementById('musicDelete').onclick=()=>{if(writable()&&current){deleteConfirm.hidden=false;document.getElementById('musicDeleteCancel').focus();}};
  document.getElementById('musicDeleteCancel').onclick=()=>{deleteConfirm.hidden=true;document.getElementById('musicDelete').focus();};
  document.getElementById('musicDeleteConfirmButton').onclick=()=>editorAction('delete');
  document.getElementById('musicUnpublish').onclick=()=>editorAction('unpublish');
  async function editorAction(kind) {
    if(!writable()||!current)return;
    const selected=current;saving=true;saveError.hidden=true;actionStatus.textContent='';controls();
    saveController=new AbortController();const timeout=setTimeout(()=>saveController.abort(),25000);
    try {
      if(kind==='delete'){
        await request('/admin/api/music/'+encodeURIComponent(selected.id),{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({revision:selected.revision}),signal:saveController.signal});
        needsRefresh=true;modal.close();await load();
      } else {
        const next=items.map(item=>({...item,published:item.id===selected.id?false:item.published}));
        const layout=[...next.filter(item=>item.published),...next.filter(item=>!item.published)];
        const data=await request('/admin/api/music/layout',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({items:layout.map(item=>({id:item.id,revision:item.revision,published:item.published}))}),signal:saveController.signal});
        items=data.items;current=items.find(item=>item.id===selected.id);render();deleteConfirm.hidden=true;actionStatus.textContent='已下架';
      }
    } catch(e) {
      needsRefresh=true;modal.close();error.textContent=(e.name==='AbortError'?'操作超时，请确认当前结果。':e.message)+' 正在重新读取乐库。';error.hidden=false;await load(true);
    } finally {clearTimeout(timeout);saveController=null;saving=false;controls();}
  }
  form.addEventListener('submit',async e=>{
    e.preventDefault();if(!writable())return;const start=startSeconds(),duration=preview&&preview.duration;if(!Number.isFinite(start)||start<0||start>=1200||(Number.isFinite(duration)&&start>=duration)){startInput.setCustomValidity('起播时间必须小于音乐时长。');startInput.reportValidity();return;}saving=true;save.disabled=true;saveError.hidden=true;controls();
    const item={...(current||{}),id:current?current.id:'',revision:current?current.revision:0,published:current?current.published:false,sort_order:current?current.sort_order:Math.min(9999,Math.max(-1,...items.map(v=>v.sort_order))+1)};['name','artist'].forEach(key=>{item[key]=form.elements[key].value.trim();});
    const linked=form.elements.audio_mode.value==='link';item.audio_link=linked?form.elements.audio_link.value.trim():'';
    item.start_offset_ms=Math.round(start*1000);
    item.duration_ms=linked?Math.round(Number(form.elements.duration_seconds.value||0)*1000):0;
    const data=new FormData();data.set('metadata',JSON.stringify(item));['audio','cover'].forEach(key=>{const f=form.elements[key].files[0];if(f&&(key!=='audio'||!linked))data.set(key,f);});
    saveController=new AbortController();const timeout=setTimeout(()=>saveController.abort(),60000);
    try {await request('/admin/api/music'+(current?'/'+encodeURIComponent(current.id):''),{method:current?'PUT':'POST',body:data,signal:saveController.signal});needsRefresh=true;modal.close();await load();}
    catch(e){saveError.textContent=e.name==='AbortError'?'保存超时，请刷新确认结果后再操作。':e.message;saveError.hidden=false;saveError.scrollIntoView({block:'nearest'});}
    finally{clearTimeout(timeout);saveController=null;saving=false;save.disabled=false;controls();}
  });
  document.getElementById('musicClose').onclick=()=>{if(!saving)modal.close();};modal.addEventListener('cancel',e=>{if(saving)e.preventDefault();});
  document.getElementById('musicAdd').onclick=()=>edit(null);document.getElementById('musicRefresh').onclick=()=>{if(!saving&&!arranging)load();};
  window.addEventListener('pagehide',()=>{disposed=true;stopPreview();version++;if(controller)controller.abort();if(saveController)saveController.abort();if(layoutController)layoutController.abort();closeTrackPreview();});
  document.addEventListener('visibilitychange',()=>{if(document.hidden){preview?.pause();trackPreview?.audio.pause();}});load();
})();
