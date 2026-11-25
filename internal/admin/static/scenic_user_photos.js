const ScenicUserPhotos = (() => {
  const el = id => document.getElementById(id);
  const {node, button, tile, time} = WorkoutPhotos;
  const viewer = WorkoutPhotos.viewer();
  let spot, cursor = '', items = [], request, generation = 0, saving = false, pageCount = 0;
  const base = () => '/admin/api/scenic-spots/' + encodeURIComponent(spot) + '/user-photos';
  const url = p => base() + '/' + encodeURIComponent(p.id);
  const state = p => p.included ? '已收录' : '待收录';
  const canWrite = () => AdminAccess.can('scenic_spots', true);
  async function source(photo) {
    const wasOpen = viewer.open, version = generation;
    viewer.close();
    try { await AdminTrackEditor.open(photo.track_id, {waitForHidden:true}); }
    catch { if (generation === version) el('userScenicMessage').textContent = '轨迹详情加载失败'; }
    finally { if (wasOpen && generation === version && items.includes(photo)) preview(items.indexOf(photo)); }
  }
  function preview(index) {
    const p = items[index];
    if (!p) return;
    const actions = [];
    if (AdminAccess.can('tracks', false)) actions.push(button('来源轨迹', 'btn btn-outline-secondary', () => source(p)));
    if (canWrite()) actions.push(button(p.included ? '移出景点' : '收录到景点', p.included ? 'btn btn-outline-secondary' : 'btn btn-primary', () => decide(p)));
    viewer.render({url:url(p) + '/image', title:state(p) + ' · 用户实拍', detail:'轨迹 ' + p.track_id + ' · ' + time(p),
      index, count:items.length, move:preview, actions, busy:saving});
  }
  function card(p) {
    const box = node('div', 'photo-card');
    const caption = node('div', 'photo-caption');
    const sourceLabel = '轨迹 ' + p.track_id;
    caption.append(node('span', 'photo-state', state(p)), AdminAccess.can('tracks', false)
      ? button(sourceLabel, 'photo-source', () => source(p)) : node('span', 'photo-source', sourceLabel));
    box.append(tile(url(p) + '/image', '查看' + state(p) + '照片', () => preview(items.indexOf(p))), caption);
    return box;
  }
  async function decide(p) {
    if (saving || !canWrite() || !items.includes(p)) return;
    const version = generation;
    saving = true; preview(items.indexOf(p));
    try {
      const response = await fetch(url(p), {method:'POST', headers:{'Content-Type':'application/json'},
        body:JSON.stringify({revision:p.revision, scenic_choice:p.included ? '-' : spot})});
      if (version !== generation) return;
      if (!response.ok) {
        if (response.status === 409 || response.status === 404) {
          const pages = pageCount;
          viewer.close();
          await load(false, pages);
        }
        throw Error('收录状态更新失败，请核对照片后重试');
      }
      p.included = !p.included; p.revision++;
      const index = items.indexOf(p);
      el('userScenicItems').children[index]?.replaceWith(card(p));
      el('userScenicMessage').textContent = '';
    } catch (e) {
      if (version === generation) { viewer.close(); el('userScenicMessage').textContent = e.message; }
    } finally {
      if (version === generation) { saving = false; if (viewer.open) preview(items.indexOf(p)); }
    }
  }
  async function load(append, pages = 1) {
    request?.abort();
    const current = request = new AbortController();
    el('userScenicMore').disabled = true;
    try {
      let after = append ? cursor : '';
      const fresh = [];
      let loaded = 0;
      do {
        const res = await fetch(base() + '?cursor=' + encodeURIComponent(after), {signal:current.signal});
        if (!res.ok) throw Error('用户实拍加载失败');
        const {data} = await res.json();
        if (current.signal.aborted) return;
        fresh.push(...data.items); after = data.next_cursor || ''; loaded++;
      } while (after && loaded < pages);
      if (!append) { items = []; pageCount = 0; el('userScenicItems').replaceChildren(); }
      const ids = new Set(items.map(p => p.id));
      const added = fresh.filter(p => !ids.has(p.id));
      items.push(...added); pageCount += loaded; cursor = after;
      el('userScenicItems').append(...added.map(card));
      el('userScenicMore').hidden = !cursor;
      el('userScenicMessage').textContent = items.length ? '' : '暂无已审核的用户实拍';
    } catch (e) {
      if (!current.signal.aborted) {
        if (!append) { items = []; pageCount = 0; cursor = ''; el('userScenicItems').replaceChildren(); }
        el('userScenicMessage').textContent = '用户实拍加载失败';
        el('userScenicMore').textContent = '重新加载'; el('userScenicMore').hidden = false;
      }
    } finally {
      if (!current.signal.aborted) el('userScenicMore').disabled = false;
    }
  }
  el('userScenicMore').onclick = () => { el('userScenicMore').textContent = '加载更多'; load(!!cursor); };
  return {open(id) {
    generation++; request?.abort(); viewer.close(); saving = false; spot = id; cursor = ''; items = []; pageCount = 0;
    el('userScenicPhotos').hidden = !id;
    el('userScenicItems').replaceChildren(); el('userScenicMore').hidden = true; el('userScenicMore').textContent = '加载更多';
    el('userScenicMessage').textContent = id ? '正在加载照片…' : '';
    if (id) load(false);
  }};
})();
