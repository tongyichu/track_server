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
    const picker = document.createElement('details'); picker.className = 'admin-track-label-picker';
    const summary = document.createElement('summary'); summary.className = 'admin-track-label-summary';
    const title = document.createElement('span'); title.className = 'admin-track-label-caption'; title.textContent = '景点';
    const currentName = document.createElement('span'); currentName.className = 'admin-track-label-current';
    const values = document.createElement('span'); values.className = 'admin-track-label-values';
    const add = document.createElement('span'); add.className = 'admin-track-label-add'; add.textContent = '+'; add.setAttribute('aria-hidden', 'true');
    summary.setAttribute('aria-label', '查看或设置景点'); values.append(currentName, add); summary.append(title, values);
    const panel = document.createElement('div'); panel.className = 'admin-track-label-options';
    const canWrite = !options.readOnly && AdminAccess.can('tracks', true);
    const canManual = canWrite && AdminAccess.can('scenic_spots', true) && typeof options.scenicSpotID === 'string';
    add.hidden = !canWrite;
    const actions = document.createElement('div'); actions.className = 'd-flex flex-wrap gap-2';
    const dismiss = document.createElement('button'); dismiss.type = 'button'; dismiss.className = 'btn btn-sm btn-outline-secondary';
    dismiss.textContent = '忽略'; dismiss.title = '忽略本次候选'; dismiss.dataset.adminWrite = 'tracks'; dismiss.hidden = true;
    const detail = document.createElement('a'); detail.className = 'btn btn-sm btn-link'; detail.textContent = '查看景点'; detail.target = '_blank'; detail.rel = 'noopener'; detail.hidden = true;
    actions.append(dismiss, detail);
    const body = document.createElement('div'); body.className = 'admin-track-candidates-body'; body.setAttribute('aria-live', 'polite');
    panel.append(actions, body); picker.append(summary, panel); root.appendChild(picker);
    let busy = false, sequence = 0, loaded = false, expectedSpotID = options.scenicSpotID;
    let recognitionStatus = options.recognitionStatus;
    const controller = new AbortController();
    let disposed = false, candidateAbort;
    const nearby = document.createElement('div'); nearby.className = 'admin-track-nearby'; nearby.hidden = true; panel.appendChild(nearby);
    root.showNearby = function (state) {
      if (disposed) return;
      picker.open = true;
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

    function setPrimarySpot(spot, assignedID, status) {
      if (typeof assignedID === 'string') expectedSpotID = assignedID;
      if (status) recognitionStatus = status;
      const selected = !!spot && !!spot.id;
      const statuses = { waiting: 'AI等待查询', running: 'AI正在查询', not_found: 'AI没有查到' };
      currentName.textContent = selected ? spot.name || '未命名景点' : statuses[recognitionStatus] || '未关联';
      currentName.title = currentName.textContent; currentName.classList.toggle('is-empty', !selected);
      detail.hidden = !selected;
      if (selected) detail.href = '/admin/scenic_spots.html?spot_id=' + encodeURIComponent(spot.id);
      else detail.removeAttribute('href');
    }
    root.setPrimarySpot = setPrimarySpot;
    root.dispose = function () { disposed = true; sequence++; clearTimeout(searchTimer); if (searchAbort) searchAbort.abort(); if (candidateAbort) candidateAbort.abort(); controller.abort(); };

    function state(text) { dismiss.hidden = true; const note = document.createElement('p'); note.className = 'text-secondary mb-0'; note.textContent = text; body.replaceChildren(note); }
    function setBusy(value) {
      busy = value;
      if (value && candidateAbort) { ++sequence; candidateAbort.abort(); candidateAbort = null; }
      root.querySelectorAll('button').forEach(button => { button.disabled = value || button.dataset.unavailable === 'true'; }); syncManual();
    }
    function placeTag(name, unavailable) {
      const tag = document.createElement('button'); tag.type = 'button'; tag.className = 'btn btn-sm btn-outline-primary admin-track-place-tag';
      tag.textContent = name || '未命名景点'; tag.title = tag.textContent;
      tag.dataset.unavailable = String(unavailable); tag.disabled = busy || unavailable;
      if (canWrite) tag.dataset.adminWrite = 'tracks';
      return tag;
    }
    let searchTimer, searchAbort, searchSequence = 0, searchedQuery = '';
    const manual = document.createElement('div'); manual.className = 'admin-track-place-manual'; manual.hidden = !canManual;
    const input = document.createElement('input'); input.type = 'search'; input.className = 'form-control'; input.maxLength = 160;
    input.placeholder = '搜索或输入景点名称'; input.setAttribute('aria-label', '搜索或输入景点名称'); input.dataset.adminWrite = 'tracks';
    const matches = document.createElement('div'); matches.className = 'admin-track-place-matches'; matches.setAttribute('aria-live', 'polite');
    const createButton = document.createElement('button'); createButton.type = 'button'; createButton.className = 'btn btn-sm btn-outline-primary'; createButton.textContent = '新建并关联'; createButton.dataset.adminWrite = 'tracks'; createButton.hidden = true;
    manual.append(input, matches, createButton); panel.prepend(manual);
    function syncManual() {
      input.disabled = busy || !canManual;
      createButton.hidden = !canManual || !searchedQuery || searchedQuery !== input.value.trim();
      createButton.disabled = busy || createButton.hidden;
    }
    function searchNote(text) { const note = document.createElement('p'); note.className = 'text-secondary mb-0'; note.textContent = text; matches.replaceChildren(note); }
    async function searchCatalog() {
      const query = input.value.trim();
      const current = ++searchSequence;
      if (searchAbort) searchAbort.abort();
      searchedQuery = ''; syncManual(); matches.replaceChildren();
      if (!query || disposed || !canManual) return;
      const abort = new AbortController(); searchAbort = abort;
      const timeout = setTimeout(() => abort.abort(), 10000);
      searchNote('正在搜索…');
      try {
        const data = await request('/scenic-spots?view=picker&q=' + encodeURIComponent(query) + '&limit=20', { signal: abort.signal });
        if (disposed || current !== searchSequence) return;
        searchedQuery = query;
        const items = data.items || [];
        matches.replaceChildren();
        const tags = document.createElement('div'); tags.className = 'admin-track-place-tags';
        items.forEach(spot => {
          const tag = placeTag(spot.name, !canManual);
          tag.title = [spot.name].concat(spot.related_city_labels || []).join(' · ');
          tag.addEventListener('click', () => assignManual({ scenic_spot_id: spot.id }, spot.name)); tags.appendChild(tag);
        });
        if (tags.childElementCount) matches.appendChild(tags); else searchNote('目录中没有匹配景点');
        if (data.has_more) { const note = document.createElement('p'); note.className = 'text-secondary mb-0'; note.textContent = '匹配较多，可输入更完整的名称'; matches.appendChild(note); }
      } catch (error) {
        if (!disposed && current === searchSequence) {
          searchNote('搜索失败，请重试');
          const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-sm btn-outline-secondary'; retry.textContent = '重试'; retry.addEventListener('click', searchCatalog); matches.appendChild(retry);
        }
      } finally { clearTimeout(timeout); if (!disposed && current === searchSequence) syncManual(); }
    }
    input.addEventListener('input', event => {
      ++searchSequence; if (searchAbort) searchAbort.abort(); clearTimeout(searchTimer);
      searchedQuery = ''; syncManual(); matches.replaceChildren();
      if (!event.isComposing) searchTimer = setTimeout(searchCatalog, 300);
    });
    input.addEventListener('compositionend', () => { clearTimeout(searchTimer); searchTimer = setTimeout(searchCatalog, 300); });
    input.addEventListener('keydown', event => { if (event.key === 'Enter' && !event.isComposing) { event.preventDefault(); clearTimeout(searchTimer); searchCatalog(); } });
    async function assignManual(payload, name) {
      if (disposed || busy || !canManual) return;
      setBusy(true);
      try {
        const data = await fetchData('/tracks/' + encodeURIComponent(trackID) + '/scenic-spot', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(Object.assign({ expected_spot_id: expectedSpotID }, payload))
        });
        if (disposed) return;
        setPrimarySpot(data.primary_scenic_spot, data.scenic_spot_id);
        AdminUI.alert('已关联景点：' + name, 'success');
        ++searchSequence; if (searchAbort) searchAbort.abort(); clearTimeout(searchTimer);
        input.value = ''; searchedQuery = ''; matches.replaceChildren();
        picker.open = false; summary.focus();
        if (typeof options.onAssigned === 'function') {
          try { await options.onAssigned(data); }
          catch (_) { AdminUI.alert('景点已关联，列表刷新失败，请手动刷新', 'warning'); }
        }
      } catch (error) {
        if (disposed || error.name === 'AbortError') return;
        AdminUI.alert(error.status === 409 ? '景点关联已变化，请重新打开详情后选择' : '关联失败：' + error.message + '；结果不确定时请重新打开详情核实', 'danger');
      } finally { if (!disposed) setBusy(false); }
    }
    createButton.addEventListener('click', async () => {
      const name = input.value.trim();
      if (disposed || busy || createButton.disabled || !name) return;
      const originalSpotID = expectedSpotID;
      setBusy(true);
      try {
        const confirmed = await AdminUI.confirm({ title: '新建景点', message: '将“' + name + '”新增到景点目录并立即关联此轨迹？', confirmText: '新建并关联' });
        setBusy(false);
        if (confirmed && !disposed && originalSpotID === expectedSpotID && name === input.value.trim()) await assignManual({ manual_name: name }, name);
      } finally { if (!disposed) setBusy(false); }
    });
    setPrimarySpot(track.primary_scenic_spot); syncManual();
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
        if (!disposed) { picker.open = false; summary.focus(); }
      } catch (error) {
        if (disposed || error.name === 'AbortError') return;
        if (error.status === 404 || error.status === 409) { loaded = false; state('当前候选或轨迹已变化，请重新打开详情'); }
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
        if (!disposed) { picker.open = false; summary.focus(); }
      } catch (error) {
        if (disposed || error.name === 'AbortError') return;
        if (error.status === 404 || error.status === 409) { loaded = false; state('当前候选或轨迹已变化，请重新打开详情'); }
        AdminUI.alert('处理失败：' + error.message, 'danger');
      } finally { setBusy(false); }
    }

    async function load() {
      if (disposed || busy || candidateAbort) return;
      const current = ++sequence;
      const abort = new AbortController(); candidateAbort = abort;
      const timeout = setTimeout(() => abort.abort(), 10000);
      state('正在读取景点识别结果…');
      try {
        const data = await request('/tracks/' + encodeURIComponent(trackID) + '/place-candidates', { signal: abort.signal });
        if (current !== sequence) return;
        body.replaceChildren(); loaded = true;
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
        state(error.status === 404 ? '暂无候选景点' : error.status === 409 ? '候选已过期，等待后台查询；也可手动选择' : '景点候选暂不可用，请关闭后重试');
      } finally { clearTimeout(timeout); if (candidateAbort === abort) candidateAbort = null; }
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

    picker.addEventListener('toggle', () => {
      if (disposed) return;
      if (!picker.open) {
        ++searchSequence; clearTimeout(searchTimer); if (searchAbort) searchAbort.abort();
        if (candidateAbort) { ++sequence; candidateAbort.abort(); candidateAbort = null; }
        return;
      }
      if (!loaded && !busy) load();
      if (canManual && input.value.trim() && searchedQuery !== input.value.trim()) searchCatalog();
    });
    return root;
  }

  window.AdminTrackPlaceCandidates = { create: create };
})();
