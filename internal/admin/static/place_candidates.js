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
    const root = document.createElement('section');
    root.className = 'admin-track-candidates';
    const heading = document.createElement('div'); heading.className = 'admin-track-candidates-heading';
    const title = document.createElement('h3'); title.textContent = '景点识别';
    const refresh = document.createElement('button'); refresh.type = 'button'; refresh.className = 'btn btn-sm btn-outline-secondary'; refresh.textContent = '刷新识别结果';
    heading.append(title, refresh);
    const body = document.createElement('div'); body.className = 'admin-track-candidates-body'; body.setAttribute('aria-live', 'polite');
    root.append(heading, body);
    let busy = false, sequence = 0;

    function state(text) { const note = document.createElement('p'); note.className = 'text-secondary mb-0'; note.textContent = text; body.replaceChildren(note); }
    function setBusy(value) { busy = value; root.querySelectorAll('button').forEach(button => { button.disabled = value || button.dataset.unavailable === 'true'; }); }
    async function act(path, payload, message) {
      if (busy || options.readOnly || !AdminAccess.can('tracks', true)) return;
      setBusy(true);
      try {
        await request(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload || {}) });
        state(message); AdminUI.alert(message, 'success');
        if (typeof options.onChanged === 'function') {
          try { await options.onChanged(); }
          catch (_) { AdminUI.alert('景点处理已生效，列表刷新失败，请手动刷新', 'warning'); }
        }
      } catch (error) {
        if (error.status === 404 || error.status === 409) state('当前候选或轨迹数据已变化，请刷新识别结果后重试');
        AdminUI.alert('处理失败：' + error.message, 'danger');
      }
      finally { setBusy(false); }
    }

    async function load() {
      if (busy) return;
      const current = ++sequence;
      setBusy(true); state('正在读取景点识别结果…');
      try {
        const data = await request('/tracks/' + encodeURIComponent(trackID) + '/place-candidates');
        if (current !== sequence) return;
        body.replaceChildren();
        const note = document.createElement('p'); note.className = 'text-secondary';
        note.textContent = '这条轨迹围住了多个景点，请确认其所属景点。确认后立即生效。'; body.appendChild(note);
        (data.places || []).forEach(place => {
          const row = document.createElement('div'); row.className = 'admin-track-candidate';
          const info = document.createElement('div'); info.className = 'admin-track-candidate-info';
          const name = document.createElement('strong'); name.textContent = place.name || '未命名景点';
          const meta = document.createElement('div'); meta.className = 'text-secondary';
          meta.textContent = [place.category, options.cityNames && options.cityNames[place.city_code]].filter(Boolean).join(' · ');
          info.append(name, meta); row.appendChild(info);
          if (!options.readOnly && AdminAccess.can('tracks', true)) {
            const actions = document.createElement('div'); actions.dataset.adminWrite = 'tracks';
            const confirm = document.createElement('button'); confirm.type = 'button'; confirm.className = 'btn btn-sm btn-outline-primary'; confirm.textContent = '确认关联';
            confirm.dataset.unavailable = String(!place.name || !place.name.trim());
            confirm.addEventListener('click', () => act('/place-candidates/' + encodeURIComponent(trackID) + '/confirm', { place_id: place.id }, '已关联景点：' + place.name));
            actions.appendChild(confirm); row.appendChild(actions);
          }
          body.appendChild(row);
        });
        const footer = document.createElement('div'); footer.className = 'admin-track-candidates-heading mt-3';
        const identified = document.createElement('span'); identified.className = 'text-secondary';
        const date = new Date(data.updated_at); identified.textContent = Number.isNaN(date.getTime()) ? '' : '识别时间：' + date.toLocaleString('zh-CN', { hour12: false });
        footer.appendChild(identified);
        if (!options.readOnly && AdminAccess.can('tracks', true)) {
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
        state(error.status === 404 ? '暂无待确认的景点候选' : error.status === 409 ? '轨迹数据已变化，当前识别结果已过期，请等待重新识别' : '景点识别结果暂不可用，请稍后刷新');
      } finally { if (current === sequence) setBusy(false); }
    }
    refresh.addEventListener('click', load);
    load();
    return root;
  }

  window.AdminTrackPlaceCandidates = { create: create };
})();
