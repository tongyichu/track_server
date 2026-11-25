(() => {
  const el = id => document.getElementById(id);
  const {node, button, tile, time} = WorkoutPhotos;
  const viewer = WorkoutPhotos.viewer();
  const reasonDialog = el('photoReasonDialog');
  const selected = new Set();
  const canWrite = () => AdminAccess.can('track_submissions', true);
  const endpoint = '/admin/api/track-photos';
  const image = p => endpoint + '/' + encodeURIComponent(p.id) + '/image';
  let rows = [], cursors = [''], page = 0, next = '', currentId = '', rejection = [];
  let request, writing = false, loading = false;

  function controls() {
    const busy = writing || loading;
    el('photoRefresh').disabled = busy;
    el('photoFilter').disabled = writing;
    el('routesTab').disabled = writing;
    el('photoBulk').hidden = !selected.size || !canWrite() || loading;
    el('photoSelection').textContent = '已选 ' + selected.size + ' 张';
    el('photoApprove').hidden = el('photoFilter').value === 'approved';
    el('photoReject').hidden = el('photoFilter').value === 'rejected';
    for (const id of ['photoApprove','photoReject','photoClear']) el(id).disabled = busy;
    el('photoPager').hidden = !rows.length || (!page && !next);
    el('photoPageNumber').textContent = '第 ' + (page + 1) + ' 页';
    el('photoPrev').disabled = busy || !page;
    el('photoNext').disabled = busy || !next;
    el('photoPanel').toggleAttribute('data-empty', !rows.length);
  }

  function preview(index) {
    const p = rows[index];
    if (!p) return viewer.close();
    currentId = p.id;
    const actions = [];
    if (canWrite()) {
      if (p.status !== 'rejected') actions.push(button('驳回', 'btn btn-outline-danger', () => askReason([p])));
      if (p.status !== 'approved') actions.push(button('通过', 'btn btn-primary', () => review([p], 'approved')));
    }
    const detail = [time(p), p.status === 'rejected' && p.reason].filter(Boolean).join(' · ');
    viewer.render({url:image(p), title:'轨迹 ' + p.track_id, detail, index, count:rows.length,
      move:preview, actions, busy:writing || loading});
  }

  function render() {
    const groups = new Map();
    for (const p of rows) {
      if (!groups.has(p.track_id)) groups.set(p.track_id, []);
      groups.get(p.track_id).push(p);
    }
    const content = document.createDocumentFragment();
    for (const [track, photos] of groups) {
      const group = node('section', 'photo-group');
      const heading = node('h3', '', '轨迹 ' + track);
      heading.append(node('small', '', photos.length + ' 张'));
      const grid = node('div', 'photo-grid');
      for (const p of photos) {
        const card = node('div', 'photo-card');
        card.append(tile(image(p), '查看 ' + time(p) + ' 的照片', () => preview(rows.indexOf(p))));
        if (canWrite()) {
          const label = node('label','photo-select'), check = node('input');
          check.type = 'checkbox';
          check.checked = selected.has(p.id);
          check.disabled = loading || writing;
          check.setAttribute('aria-label', '选择 ' + time(p) + ' 的照片');
          check.onchange = () => { if (check.checked) selected.add(p.id); else selected.delete(p.id); controls(); };
          label.append(check); card.append(label);
        }
        card.append(node('div','photo-caption',AdminTime.short(p.taken_at_ms, true)));
        if (p.status === 'rejected' && p.reason) card.append(node('div','photo-caption',p.reason));
        grid.append(card);
      }
      group.append(heading, grid); content.append(group);
    }
    el('photoList').replaceChildren(content);
    controls();
  }

  async function load(index, continuePreview = false) {
    request?.abort();
    const current = request = new AbortController();
    if (!continuePreview) viewer.close();
    reasonDialog.close();
    selected.clear(); rows = []; next = ''; loading = true; page = index;
    el('photoMessage').textContent = '正在加载照片…';
    render();
    const status = el('photoFilter').value;
    try {
      for (;;) {
        const params = new URLSearchParams({status, cursor:cursors[page] || ''});
        const response = await fetch(endpoint + '?' + params, {signal:current.signal});
        if (!response.ok) throw Error();
        const {data} = await response.json();
        if (current.signal.aborted) return false;
        rows = data.items || []; next = data.next_cursor || '';
        el('photoPending').textContent = data.pending;
        if (rows.length || !page) break;
        cursors.length = page--;
      }
      el('photoMessage').textContent = rows.length ? '' : ({pending:'暂无待审照片', approved:'暂无已通过的照片', rejected:'暂无已驳回的照片'})[status];
      return true;
    } catch (e) {
      if (!current.signal.aborted) {
        viewer.close();
        el('photoMessage').textContent = '照片加载失败，请刷新重试';
      }
      return false;
    } finally {
      if (!current.signal.aborted) { loading = false; render(); }
    }
  }

  function askReason(targets) {
    if (writing || loading || !targets.length || !canWrite()) return;
    rejection = targets;
    el('photoReason').value = '';
    el('photoReason').setCustomValidity('');
    el('photoReasonTitle').textContent = '驳回 ' + targets.length + ' 张照片';
    reasonDialog.showModal(); el('photoReason').focus();
  }
  async function review(targets, status, reason = '') {
    if (loading || writing || !canWrite() || !targets.length || targets.some(p => !rows.includes(p))) return;
    const index = rows.findIndex(p => p.id === currentId);
    const successor = rows[index + 1]?.id;
    const previous = rows[index - 1]?.id;
    writing = true;
    render();
    if (viewer.open) preview(index);
    let failed = 0;
    try {
      for (const p of targets) {
        try {
          const response = await fetch(endpoint + '/' + encodeURIComponent(p.id) + '/review', {
            method:'POST', headers:{'Content-Type':'application/json'},
            body:JSON.stringify({status, revision:p.revision, ...(status === 'rejected' ? {reason} : {})})
          });
          if (!response.ok) failed++;
        } catch { failed++; }
      }
      const ok = await load(page, viewer.open);
      if (!ok) return;
      if (failed) {
        viewer.close();
        el('photoMessage').textContent = (targets.length - failed) + ' 张已处理，' + failed + ' 张未处理，请按最新状态重试';
      } else if (viewer.open && rows.length) {
        let nextIndex = rows.findIndex(p => p.id === successor);
        if (nextIndex < 0) nextIndex = Math.min(Math.max(0, index), rows.length - 1);
        if (!successor && rows[nextIndex]?.id === previous && next) {
          cursors[page + 1] = next;
          if (!await load(page + 1, true)) return;
          nextIndex = 0;
        }
        if (viewer.open) preview(nextIndex);
      } else if (!rows.length) viewer.close();
      else el('photoMessage').textContent = '已处理 ' + targets.length + ' 张照片';
    } finally {
      writing = false;
      render();
      if (viewer.open) preview(rows.findIndex(p => p.id === currentId));
    }
  }
  function tab(photos) {
    if (writing) return;
    request?.abort(); selected.clear(); viewer.close(); reasonDialog.close();
    loading = false;
    el('routesPanel').hidden = photos; el('photoPanel').hidden = !photos;
    for (const [id, active] of [['photosTab', photos], ['routesTab', !photos]]) {
      el(id).classList.toggle('btn-primary', active);
      el(id).classList.toggle('btn-outline-secondary', !active);
      el(id).setAttribute('aria-pressed', String(active));
    }
    if (photos) load(page);
  }
  const selectedRows = () => rows.filter(p => selected.has(p.id));
  el('photosTab').onclick = () => tab(true);
  el('routesTab').onclick = () => tab(false);
  el('photoFilter').onchange = () => { cursors = ['']; load(0); };
  el('photoRefresh').onclick = () => load(page);
  el('photoPrev').onclick = () => load(page - 1);
  el('photoNext').onclick = () => { cursors[page + 1] = next; load(page + 1); };
  el('photoClear').onclick = () => { selected.clear(); render(); };
  el('photoApprove').onclick = () => review(selectedRows(), 'approved');
  el('photoReject').onclick = () => askReason(selectedRows());
  el('cancelReason').onclick = () => reasonDialog.close();
  el('photoReason').oninput = () => el('photoReason').setCustomValidity('');
  el('photoReasonForm').onsubmit = event => {
    event.preventDefault();
    const reason = el('photoReason').value.trim();
    if (!reason) { el('photoReason').setCustomValidity('请填写驳回原因'); el('photoReason').reportValidity(); return; }
    const targets = rejection;
    reasonDialog.close(); review(targets, 'rejected', reason);
  };
  reasonDialog.onclose = () => { if (!reasonDialog.open) { rejection = []; el('photoReason').value = ''; } };
  fetch(endpoint + '?counts=1').then(r => r.ok ? r.json() : null)
    .then(body => { if (!request) el('photoPending').textContent = body?.data?.pending ?? '—'; })
    .catch(() => { if (!request) el('photoPending').textContent = '—'; });
})();
