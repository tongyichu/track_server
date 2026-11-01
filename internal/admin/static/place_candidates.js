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

  function kindLabel(relation) {
    if (!relation) return '';
    if (relation.kind === 'enclosed_poi') return '围住景点';
    if (relation.kind === 'nearby') return '途经附近 · 命中 ' + Number(relation.matched_samples || 0) + '/' + Number(relation.sample_count || 0) + ' 个采样';
    return relation.kind;
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
    heading.append(title, refresh);
    const body = document.createElement('div'); body.className = 'admin-track-candidates-body'; body.setAttribute('aria-live', 'polite');
    root.append(heading, body);
    let busy = false, sequence = 0;

    const spot = track.primary_scenic_spot;
    if (spot && spot.id) {
      const primary = document.createElement('p'); primary.className = 'admin-track-primary-spot';
      const label = document.createElement('span'); label.textContent = '当前主景点：';
      const tag = document.createElement('a');
      tag.className = 'badge bg-green-lt text-green admin-track-spot-tag';
      tag.textContent = spot.name || spot.id;
      tag.href = '/admin/scenic_spots.html?spot_id=' + encodeURIComponent(spot.id);
      tag.target = '_blank';
      tag.rel = 'noopener';
      tag.title = '打开景点详情';
      primary.append(label, tag);
      body.appendChild(primary);
    }

    function state(text) { const note = document.createElement('p'); note.className = 'text-secondary mb-0'; note.textContent = text; body.querySelectorAll(':scope > :not(.admin-track-primary-spot)').forEach(node => node.remove()); body.appendChild(note); }
    function setBusy(value) { busy = value; root.querySelectorAll('button').forEach(button => { button.disabled = value || button.dataset.unavailable === 'true'; }); }
    async function act(path, payload, message) {
      if (busy || !canWrite) return;
      setBusy(true);
      try {
        await request(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload || {}) });
        state(message); AdminUI.alert(message, 'success');
        if (typeof options.onChanged === 'function') {
          try { await options.onChanged(); }
          catch (_) { AdminUI.alert('景点处理已生效，列表刷新失败，请手动刷新', 'warning'); }
        }
      } catch (error) {
        if (error.status === 404 || error.status === 409) state('当前候选或轨迹数据已变化，请重新识别后重试');
        AdminUI.alert('处理失败：' + error.message, 'danger');
      }
      finally { setBusy(false); }
    }

    // 确认景点后询问是否用景点名作为轨迹标题；同意后同步表单中的标题输入框，
    // 避免之后保存表单时把新标题覆盖回旧值。
    async function offerTitleUpdate(place) {
      const name = String(place.name || '').trim();
      if (!name) return;
      const current = String((options.track && options.track.title) || '').trim();
      if (current === name) return;
      let confirmed = false;
      try {
        confirmed = await AdminUI.confirm({ title: '更新轨迹标题', message: '是否将轨迹标题改为「' + name + '」？确认后仍可在下方表单中继续调整。', confirmText: '改标题' });
      } catch (_) { confirmed = false; }
      if (!confirmed) return;
      try {
        await request('/tracks/' + encodeURIComponent(trackID), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ title: name }) });
      } catch (error) {
        AdminUI.alert('标题更新失败：' + error.message, 'danger');
        return;
      }
      if (options.track) options.track.title = name;
      const form = root.closest('form');
      const input = form && form.querySelector('[name="title"]');
      if (input) {
        input.value = name;
        input.dispatchEvent(new Event('input', { bubbles: true }));
        input.dispatchEvent(new Event('change', { bubbles: true }));
      }
      AdminUI.alert('标题已更新为：' + name, 'success');
    }

    async function confirmPlace(place) {
      if (busy || !canWrite) return;
      setBusy(true);
      try {
        await request('/place-candidates/' + encodeURIComponent(trackID) + '/confirm', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ place_id: place.id }) });
        AdminUI.alert('已关联景点：' + place.name, 'success');
        await offerTitleUpdate(place);
        if (typeof options.onChanged === 'function') {
          try { await options.onChanged(); }
          catch (_) { AdminUI.alert('景点处理已生效，列表刷新失败，请手动刷新', 'warning'); }
        }
      } catch (error) {
        if (error.status === 404 || error.status === 409) state('当前候选或轨迹数据已变化，请重新识别后重试');
        AdminUI.alert('处理失败：' + error.message, 'danger');
      } finally { setBusy(false); }
    }

    async function load() {
      if (busy) return;
      const current = ++sequence;
      setBusy(true); state('正在读取景点识别结果…');
      try {
        const data = await request('/tracks/' + encodeURIComponent(trackID) + '/place-candidates');
        if (current !== sequence) return;
        body.querySelectorAll(':scope > :not(.admin-track-primary-spot)').forEach(node => node.remove());
        const note = document.createElement('p'); note.className = 'text-secondary';
        note.textContent = '识别到以下候选景点（含围住与途经附近），请人工确认其所属景点，确认后立即生效。'; body.appendChild(note);
        const kindByPlace = new Map();
        (data.relations || []).forEach(relation => { if (!kindByPlace.has(relation.place_id)) kindByPlace.set(relation.place_id, relation); });
        (data.places || []).forEach(place => {
          const row = document.createElement('div'); row.className = 'admin-track-candidate';
          const info = document.createElement('div'); info.className = 'admin-track-candidate-info';
          const name = document.createElement('strong'); name.textContent = place.name || '未命名景点';
          const kind = document.createElement('span'); kind.className = 'badge bg-azure-lt text-azure ms-1'; kind.textContent = kindLabel(kindByPlace.get(place.id));
          name.appendChild(kind);
          const meta = document.createElement('div'); meta.className = 'text-secondary';
          meta.textContent = [place.category, options.cityNames && options.cityNames[place.city_code]].filter(Boolean).join(' · ');
          info.append(name, meta); row.appendChild(info);
          if (canWrite) {
            const actions = document.createElement('div'); actions.dataset.adminWrite = 'tracks';
            const confirm = document.createElement('button'); confirm.type = 'button'; confirm.className = 'btn btn-sm btn-outline-primary'; confirm.textContent = '确认关联';
            confirm.dataset.unavailable = String(!place.name || !place.name.trim());
            confirm.addEventListener('click', () => confirmPlace(place));
            actions.appendChild(confirm); row.appendChild(actions);
          }
          body.appendChild(row);
        });
        const footer = document.createElement('div'); footer.className = 'admin-track-candidates-heading mt-3';
        const identified = document.createElement('span'); identified.className = 'text-secondary';
        const date = new Date(data.updated_at); identified.textContent = Number.isNaN(date.getTime()) ? '' : '识别时间：' + date.toLocaleString('zh-CN', { hour12: false });
        footer.appendChild(identified);
        if (canWrite) {
          const dismiss = document.createElement('button'); dismiss.type = 'button'; dismiss.className = 'btn btn-sm btn-outline-secondary'; dismiss.textContent = '忽略本次候选'; dismiss.dataset.adminWrite = 'tracks';
          dismiss.addEventListener('click', async () => {
            if (busy) return;
            setBusy(true);
            try {
              const confirmed = await AdminUI.confirm({ title: '忽略景点候选', message: '确认忽略这条轨迹的本次景点候选？', confirmText: '忽略候选' });
              setBusy(false);
              if (confirmed) await act('/place-candidates/' + encodeURIComponent(trackID) + '/dismiss', {}, '已忽略本次景点候选');
            } finally { setBusy(false); }
          });
          footer.appendChild(dismiss);
        }
        body.appendChild(footer);
      } catch (error) {
        if (current !== sequence) return;
        state(error.status === 404 ? '暂无待确认的景点候选，可点击“重新识别”立即执行识别' : error.status === 409 ? '轨迹数据已变化，当前识别结果已过期，请点击“重新识别”' : '景点识别结果暂不可用，请稍后刷新');
      } finally { if (current === sequence) setBusy(false); }
    }

    refresh.addEventListener('click', async () => {
      if (busy) return;
      if (!canWrite) { load(); return; }
      const current = ++sequence;
      setBusy(true);
      refresh.textContent = '识别中…';
      state('正在执行景点识别，请稍候…');
      try {
        await request('/tracks/' + encodeURIComponent(trackID) + '/place-refresh', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
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
