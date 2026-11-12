(function () {
  'use strict';

  async function request(path, options) {
    const response = await fetch('/admin/api' + path, options);
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      const error = new Error(payload.error || ('请求失败：' + response.status));
      error.status = response.status;
      throw error;
    }
    return payload.data;
  }

  function create(trackID, options) {
    options = options || {};
    const track = options.track || {};
    const root = document.createElement('section');
    root.className = 'admin-track-candidates';
    const heading = document.createElement('div'); heading.className = 'admin-track-candidates-heading';
    const title = document.createElement('h3'); title.textContent = '景点识别';
    const refresh = document.createElement('button'); refresh.type = 'button'; refresh.className = 'btn btn-sm btn-outline-secondary';
    const canWrite = !options.readOnly && AdminAccess.can('tracks', true);
    refresh.textContent = canWrite ? '重新识别' : '刷新识别结果';
    const actions = document.createElement('div'); actions.className = 'd-flex flex-wrap gap-2';
    const dismiss = document.createElement('button'); dismiss.type = 'button'; dismiss.className = 'btn btn-sm btn-outline-secondary';
    dismiss.textContent = '忽略'; dismiss.title = '忽略本次候选'; dismiss.dataset.adminWrite = 'tracks'; dismiss.hidden = true;
    actions.append(refresh, dismiss); heading.append(title, actions);
    const body = document.createElement('div'); body.className = 'admin-track-candidates-body'; body.setAttribute('aria-live', 'polite');
    root.append(heading, body);
    let busy = false, sequence = 0;
    const controller = new AbortController();
    let disposed = false;
    const nearby = document.createElement('div'); nearby.className = 'admin-track-nearby'; nearby.hidden = true; root.appendChild(nearby);
    root.showNearby = function (state) {
      if (disposed) return;
      nearby.replaceChildren(); nearby.hidden = false;
      const title = document.createElement('h4'); title.textContent = '地图附近景点'; nearby.appendChild(title);
      if (state.loading || state.error || !(state.items || []).length) {
        const note = document.createElement('p'); note.className = 'text-secondary';
        note.textContent = state.loading ? '正在查询附近景点…' : state.error || '附近未找到景点，请换个位置查询。'; nearby.appendChild(note);
      }
      const tags = document.createElement('div'); tags.className = 'admin-track-place-tags';
      (state.items || []).forEach(poi => {
        const select = placeTag(poi.name, !canWrite || typeof state.select !== 'function');
        select.title = '在地图中选择：' + poi.name;
        select.addEventListener('click', () => { if (!disposed && !busy && !select.disabled) state.select(poi); });
        tags.appendChild(select);
      });
      if (tags.childElementCount) nearby.appendChild(tags);
      if (state.error) {
        const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-sm btn-outline-secondary'; retry.textContent = '重新查询';
        retry.addEventListener('click', () => { if (!disposed) state.retry(); }); nearby.appendChild(retry);
      }
      if (state.loading) nearby.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    };
    function fetchData(path, options) { return request(path, Object.assign({}, options, { signal: controller.signal })); }

    function setPrimarySpot(spot) {
      const old = body.querySelector('.admin-track-primary-spot');
      if (old) old.remove();
      if (!spot || !spot.id) return;
      const primary = document.createElement('p'); primary.className = 'admin-track-primary-spot';
      const tag = document.createElement('a');
      tag.className = 'badge bg-green-lt text-green admin-track-spot-tag';
      tag.textContent = spot.name || '未命名景点';
      tag.href = '/admin/scenic_spots.html?spot_id=' + encodeURIComponent(spot.id);
      tag.target = '_blank';
      tag.rel = 'noopener';
      tag.title = '当前主景点，点击查看详情';
      primary.appendChild(tag);
      body.prepend(primary);
    }
    root.setPrimarySpot = setPrimarySpot;
    root.dispose = function () { disposed = true; sequence++; controller.abort(); };
    setPrimarySpot(track.primary_scenic_spot);

    function state(text) { dismiss.hidden = true; const note = document.createElement('p'); note.className = 'text-secondary mb-0'; note.textContent = text; body.querySelectorAll(':scope > :not(.admin-track-primary-spot)').forEach(node => node.remove()); body.appendChild(note); }
    function setBusy(value) { busy = value; root.querySelectorAll('button').forEach(button => { button.disabled = value || button.dataset.unavailable === 'true'; }); }
    function placeTag(name, unavailable) {
      const tag = document.createElement('button'); tag.type = 'button'; tag.className = 'btn btn-sm btn-outline-primary admin-track-place-tag';
      tag.textContent = name || '未命名景点'; tag.title = tag.textContent;
      tag.dataset.unavailable = String(unavailable); tag.disabled = busy || unavailable;
      if (canWrite) tag.dataset.adminWrite = 'tracks';
      return tag;
    }
    async function act(path, payload, message) {
      if (disposed || busy || !canWrite) return;
      setBusy(true);
      try {
        await fetchData(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload || {}) });
        if (disposed) return;
        state(message); AdminUI.alert(message, 'success');
        if (typeof options.onChanged === 'function') {
          try { await options.onChanged(); }
          catch (_) { AdminUI.alert('景点处理已生效，列表刷新失败，请手动刷新', 'warning'); }
        }
      } catch (error) {
        if (disposed || error.name === 'AbortError') return;
        if (error.status === 404 || error.status === 409) state('当前候选或轨迹数据已变化，请重新识别后重试');
        AdminUI.alert('处理失败：' + error.message, 'danger');
      }
      finally { setBusy(false); }
    }

    // 确认景点后，在标题输入框旁展示服务端按客户端标题生成规则算出的建议标题；
    // 点击标签代入输入框（随表单保存生效），不自动提交。
    function showTitleSuggestion(suggestion) {
      suggestion = String(suggestion || '').trim();
      const form = root.closest('form');
      const input = form && form.querySelector('[name="title"]');
      if (!suggestion || !input || String(input.value || '').trim() === suggestion) return;
      const old = form.querySelector('.admin-track-title-suggestion');
      if (old) old.remove();
      const wrap = document.createElement('div');
      wrap.className = 'admin-track-title-suggestion mt-1';
      const tag = document.createElement('button');
      tag.type = 'button';
      tag.className = 'badge bg-azure-lt text-azure';
      tag.textContent = '代入建议标题：' + suggestion;
      tag.title = '点击代入标题输入框，保存表单后生效';
      tag.addEventListener('click', () => {
        input.value = suggestion;
        input.dispatchEvent(new Event('input', { bubbles: true }));
        input.dispatchEvent(new Event('change', { bubbles: true }));
        wrap.remove();
        AdminUI.alert('已代入建议标题，保存表单后生效', 'success');
      });
      wrap.appendChild(tag);
      input.insertAdjacentElement('afterend', wrap);
    }

    async function confirmPlace(place) {
      if (disposed || busy || !canWrite) return;
      setBusy(true);
      try {
        const data = await fetchData('/place-candidates/' + encodeURIComponent(trackID) + '/confirm', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ place_id: place.id }) });
        if (disposed) return;
        AdminUI.alert('已关联景点：' + place.name, 'success');
        showTitleSuggestion(data && data.suggested_title);
        if (typeof options.onChanged === 'function') {
          try { await options.onChanged(); }
          catch (_) { AdminUI.alert('景点处理已生效，列表刷新失败，请手动刷新', 'warning'); }
        }
      } catch (error) {
        if (disposed || error.name === 'AbortError') return;
        if (error.status === 404 || error.status === 409) state('当前候选或轨迹数据已变化，请重新识别后重试');
        AdminUI.alert('处理失败：' + error.message, 'danger');
      } finally { setBusy(false); }
    }

    async function load() {
      if (disposed || busy) return;
      const current = ++sequence;
      setBusy(true); state('正在读取景点识别结果…');
      try {
        const data = await fetchData('/tracks/' + encodeURIComponent(trackID) + '/place-candidates');
        if (current !== sequence) return;
        body.querySelectorAll(':scope > :not(.admin-track-primary-spot)').forEach(node => node.remove());
        const tags = document.createElement('div'); tags.className = 'admin-track-place-tags';
        (data.places || []).forEach(place => {
          const tag = placeTag(place.name, !canWrite || !String(place.name || '').trim());
          tag.title = canWrite ? '关联景点：' + tag.textContent : tag.textContent;
          tag.addEventListener('click', () => confirmPlace(place)); tags.appendChild(tag);
        });
        if (tags.childElementCount) body.appendChild(tags);
        else state('暂无候选景点');
        dismiss.hidden = !canWrite || !tags.childElementCount;
      } catch (error) {
        if (current !== sequence) return;
        state(error.status === 404 ? '暂无候选景点' : error.status === 409 ? '识别结果已过期，请重新识别' : '景点识别结果暂不可用，请稍后刷新');
      } finally { if (current === sequence) setBusy(false); }
    }

    dismiss.addEventListener('click', async () => {
      if (disposed || busy || !canWrite) return;
      setBusy(true);
      try {
        const confirmed = await AdminUI.confirm({ title: '忽略景点候选', message: '确认忽略这条轨迹的本次景点候选？', confirmText: '忽略候选' });
        setBusy(false);
        if (confirmed) await act('/place-candidates/' + encodeURIComponent(trackID) + '/dismiss', {}, '已忽略本次景点候选');
      } finally { setBusy(false); }
    });

    refresh.addEventListener('click', async () => {
      if (disposed || busy) return;
      if (!canWrite) { load(); return; }
      const current = ++sequence;
      setBusy(true);
      refresh.textContent = '识别中…';
      state('正在执行景点识别，请稍候…');
      try {
        await fetchData('/tracks/' + encodeURIComponent(trackID) + '/place-refresh', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
      } catch (error) {
        if (current === sequence) AdminUI.alert(error.status === 409 ? error.message : '识别执行失败：' + error.message, error.status === 409 ? 'warning' : 'danger');
      } finally {
        if (current === sequence) { refresh.textContent = '重新识别'; setBusy(false); }
      }
      if (current === sequence) load();
    });
    load();
    return root;
  }

  window.AdminTrackPlaceCandidates = { create: create };
})();
