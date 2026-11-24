(() => {
  const el = id => document.getElementById(id);
  let page = 0, cursors = [''], next = '', request, reviewing = false;
  let rows = [];
  function showPhotos(visible) {
    el('routesPanel').hidden = visible;
    el('photoPanel').hidden = !visible;
    for (const [id, active] of [['photosTab',visible],['routesTab',!visible]]) {
      el(id).className = active ? 'btn btn-primary' : 'btn btn-outline-secondary';
      el(id).setAttribute('aria-pressed', String(active));
    }
    if (visible) load(page); else request?.abort();
  }
  function render(items) {
    const groups = new Map();
    for (const p of items) {
      if (!groups.has(p.track_id)) groups.set(p.track_id, []);
      groups.get(p.track_id).push(p);
    }
    const content = document.createDocumentFragment();
    for (const [track, photos] of groups) {
      const heading = document.createElement('h3');
      heading.textContent = track;
      const grid = document.createElement('div');
      grid.className = 'd-flex flex-wrap gap-3 mb-3';
      for (const p of photos.sort((a,b) => a.taken_at_ms - b.taken_at_ms)) {
        const card = document.createElement('label');
        card.className = 'd-flex flex-column gap-1';
        const check = document.createElement('input');
        check.type = 'checkbox'; check.value = p.id; check.setAttribute('aria-label','选择照片');
        const link = document.createElement('a');
        link.href = '/admin/api/track-photos/' + encodeURIComponent(p.id) + '/image';
        link.onclick = e => { e.preventDefault(); const dialog = el('photoPreview'); el('photoPreviewImage').src = link.href; dialog.showModal(); };
        const img = new Image(144,108); img.loading = 'lazy'; img.style.objectFit = 'cover'; img.alt = '查看大图'; img.src = link.href;
        link.append(img);
        const time = document.createElement('small'); time.textContent = AdminTime.dateTime(new Date(p.taken_at_ms).toISOString());
        card.append(link,check,time);
        if (p.reason) { const reason = document.createElement('small'); reason.textContent = p.reason; card.append(reason); }
        grid.append(card);
      }
      content.append(heading,grid);
    }
    el('photoList').replaceChildren(content);
  }
  async function load(index) {
    request?.abort(); const current = request = new AbortController();
    try {
      const params = new URLSearchParams({status: el('photoFilter').value, cursor:cursors[index] || ''});
      const res = await fetch('/admin/api/track-photos?' + params,{signal:current.signal});
      if (!res.ok) throw Error('照片列表加载失败');
      const {data} = await res.json(); if (current !== request) return;
      rows = data.items || []; page = index; next = data.next_cursor;
      el('photoPending').textContent = data.pending; render(rows);
      el('photoMessage').textContent = rows.length ? '' : '当前队列没有照片';
      el('photoPrev').disabled = !page; el('photoNext').disabled = !next;
    } catch (e) { if (!current.signal.aborted) el('photoMessage').textContent = e.message; }
  }
  async function review(status) {
    if (reviewing) return;
    const ids = new Set(Array.from(el('photoList').querySelectorAll('input:checked'), input => input.value));
    const reason = el('photoReason').value.trim();
    if (!ids.size || status === 'rejected' && !reason) { el('photoMessage').textContent = !ids.size ? '请先选择照片' : '请填写驳回原因'; return; }
    reviewing = true; let failed = 0;
    try {
      for (const p of rows.filter(p => ids.has(p.id))) {
        try {
          const response = await fetch('/admin/api/track-photos/' + encodeURIComponent(p.id) + '/review',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({status,reason,revision:p.revision})});
          if (!response.ok) failed++;
        } catch { failed++; }
      }
      cursors = ['']; await load(0);
      el('photoMessage').textContent = failed ? (ids.size - failed) + ' 张已处理，' + failed + ' 张失败，请刷新后重试' : '已处理 ' + ids.size + ' 张照片';
      if (!failed) el('photoReason').value = '';
    } finally { reviewing = false; }
  }
  el('photosTab').onclick = () => showPhotos(true); el('routesTab').onclick = () => showPhotos(false);
  el('photoFilter').onchange = () => { cursors = ['']; load(0); };
  el('photoRefresh').onclick = () => load(page);
  el('photoPrev').onclick = () => load(page-1); el('photoNext').onclick = () => { cursors[page+1]=next; load(page+1); };
  el('photoApprove').onclick = () => review('approved'); el('photoReject').onclick = () => review('rejected');
  el('photoPreviewClose').onclick = () => el('photoPreview').close();
  el('photoPreview').onclose = () => el('photoPreviewImage').removeAttribute('src');
  fetch('/admin/api/track-photos?counts=1').then(response=>response.ok?response.json():null)
    .then(body=>{ if(body?.data && !request) el('photoPending').textContent=body.data.pending; })
    .catch(()=>{ if(!request) el('photoPending').textContent='—'; });
})();