(function () {
  'use strict';

  function trackAssets(trackID, track, signal) {
    const section = document.createElement('section');
    section.className = 'admin-track-assets';
    const heading = document.createElement('h3'); heading.textContent = '轨迹预览'; section.appendChild(heading);
    const fileRow = document.createElement('div');
    fileRow.className = 'admin-track-file';
    const assetURL = asset => '/admin/api/tracks/' + encodeURIComponent(trackID) + '/assets/' + asset;
    const label = document.createElement('strong');
    label.textContent = '原始轨迹文件';
    if (String(track.raw_track_url || '').trim()) {
      const download = document.createElement('a');
      download.className = 'btn btn-sm btn-outline-primary admin-track-download';
      download.href = assetURL('raw');
      download.download = '';
      download.title = '下载原始轨迹文件';
      download.setAttribute('aria-label', '下载原始轨迹文件');
      download.innerHTML = '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3v12m-5-5 5 5 5-5M5 16v4h14v-4"/></svg>';
      const info = document.createElement('div'); info.className = 'admin-track-file-info';
      const metadata = document.createElement('span'); metadata.className = 'text-secondary'; metadata.setAttribute('role', 'status');
      const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-sm btn-link'; retry.textContent = '重试'; retry.hidden = true;
      info.append(label, metadata, retry); fileRow.append(download, info);
      let loading = false;
      section.loadMetadata = async function () {
        if (loading || signal.aborted) return;
        loading = true; retry.hidden = true; metadata.textContent = '正在读取文件信息…';
        const controller = new AbortController(), cancel = () => controller.abort();
        signal.addEventListener('abort', cancel, { once: true });
        const timer = setTimeout(cancel, 25000);
        try {
          const response = await fetch(assetURL('raw') + '?metadata=1', { signal: controller.signal });
          const result = await response.json();
          if (!response.ok) throw new Error('文件信息读取失败');
          const data = result.data || {}, bytes = data.file_size;
          if (typeof bytes !== 'number' || !Number.isFinite(bytes) || bytes < 0) throw new Error('文件大小无效');
          const units = ['B', 'KB', 'MB', 'GB']; let size = bytes, unit = 0;
          while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit++; }
          if (!signal.aborted) metadata.textContent = (data.file_type || '未知类型') + ' · ' + size.toLocaleString('zh-CN', { maximumFractionDigits: unit ? 2 : 0 }) + ' ' + units[unit];
        } catch (_) {
          if (!signal.aborted) { metadata.textContent = '文件信息读取失败'; retry.hidden = false; }
        } finally { clearTimeout(timer); signal.removeEventListener('abort', cancel); loading = false; }
      };
      retry.addEventListener('click', section.loadMetadata);
    } else {
      const missing = document.createElement('span');
      missing.className = 'text-secondary';
      missing.textContent = '缺失';
      fileRow.append(label, missing);
    }
    const grid = document.createElement('div');
    grid.className = 'admin-track-preview-grid';
    const previews = track.preview_images || {};
    [
      ['square_day', '1:1 日间', previews.square_day_url]
    ].forEach(function (asset) {
      const figure = document.createElement('figure');
      if (asset[0].startsWith('square')) figure.className = 'admin-track-preview-square';
      const caption = document.createElement('figcaption');
      caption.textContent = asset[1];
      const stage = document.createElement('div');
      stage.className = 'admin-track-preview-image';
      if (String(asset[2] || '').trim()) {
        const image = document.createElement('img');
        image.src = assetURL(asset[0]);
        image.alt = asset[1] + '预览图';
        image.loading = 'lazy';
        image.tabIndex = 0;
        image.setAttribute('role', 'button');
        image.addEventListener('keydown', function (event) {
          if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); image.click(); }
        });
        image.addEventListener('error', function () {
          const error = document.createElement('span');
          error.textContent = '加载失败';
          const retry = document.createElement('button');
          retry.type = 'button'; retry.className = 'btn btn-sm btn-outline-secondary'; retry.textContent = '重试';
          retry.addEventListener('click', function () { stage.replaceChildren(image); image.src = assetURL(asset[0]); });
          stage.replaceChildren(error, retry);
        });
        stage.appendChild(image);
      } else {
        stage.textContent = '无';
      }
      figure.append(caption, stage);
      grid.appendChild(figure);
    });
    section.append(grid, fileRow);
    return section;
  }

  function trackMetrics(track) {
    const section = document.createElement('section'); section.className = 'admin-track-metrics';
    const title = document.createElement('h3'); title.textContent = '轨迹数据';
    const list = document.createElement('dl');
    const time = value => value && !String(value).startsWith('0001-') ? AdminTime.dateTime(value, '—') : '—';
    const number = (value, unit) => value == null || !Number.isFinite(Number(value)) ? '—' : Number(value).toLocaleString('zh-CN', { maximumFractionDigits: 2 }) + unit;
    [
      ['开始时间', time(track.start_time)],
      ['结束时间', time(track.end_time)],
      ['时长', number(track.duration, ' 秒')],
      ['距离', number(track.distance == null ? null : track.distance / 1000, ' 公里')],
      ['热量', number(track.calories_burned, ' 千卡')],
      ['爬升', number(track.elevation_gain, ' 米')],
      ['平均速度', number(track.avg_speed_kmh, ' 公里/小时')]
    ].forEach(([label, value]) => {
      const group = document.createElement('div');
      const term = document.createElement('dt'); term.textContent = label;
      const detail = document.createElement('dd'); detail.textContent = value;
      group.append(term, detail); list.appendChild(group);
    });
    section.append(title, list); return section;
  }

  async function openTrackEditor(trackID, options) {
    options = options || {};
    if (!trackID) return false;
    if (!window.AdminAccess.can('tracks', false)) { window.AdminUI.alert('没有轨迹管理的查看权限', 'warning'); return false; }
    const readOnly = !window.AdminAccess.can('tracks', true);
    let track = null, isDeleted = false;
    const values = await window.AdminUI.form({
      title: (readOnly ? '查看轨迹 ' : '编辑轨迹 ') + trackID,
      wide: true, className: 'admin-track-editor', waitForHidden: true,
      load: async function (signal) {
        const trackResponse = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID), { signal: signal });
        if (!trackResponse.ok) throw new Error(trackResponse.status === 404 ? '该轨迹已删除或不存在' : '读取轨迹详情失败: ' + trackResponse.status);
        const trackPayload = ((await trackResponse.json()).data) || {};
        if (signal.aborted) throw new DOMException('Aborted', 'AbortError');
        track = trackPayload.track || trackPayload;
        const cities = Array.isArray(options.cities) ? options.cities.slice() : [];
        const cityNames = Object.create(null);
        cities.forEach(city => { cityNames[city.code] = city.name; });
        if (track.city_code && !cityNames[track.city_code]) cities.push({ code: track.city_code, name: trackPayload.city_name || '未知城市 (' + track.city_code + ')' });
        const cityFields = [{ value: '', label: '未设置' }].concat(cities.map(city => ({ value: city.code, label: city.name })));
        const typeNames = { hiking: '徒步', running: '跑步', climbing: '爬山', riding: '骑行', driving: '自驾' };
        const typeFields = Object.keys(typeNames).map(value => ({ value, label: typeNames[value] }));
        if (track.track_type && !typeNames[track.track_type]) typeFields.push({ value: track.track_type, label: track.track_type });
        const sourceFields = [{ value: '', label: '用户录制（默认）' }, { value: 'manual_seed', label: '人工导入' }];
        const coordinateFields = [{ value: '', label: '未设置' }, { value: 'WGS84', label: 'WGS84（GPS / 国际坐标）' }, { value: 'GCJ02', label: 'GCJ02（高德 / 国内坐标）' }, { value: 'BD09', label: 'BD09（百度坐标）' }];
        if (track.source_tag && !sourceFields.some(item => item.value === track.source_tag)) sourceFields.push({ value: track.source_tag, label: '历史值（' + track.source_tag + '）' });
        if (track.coordinate_system && !coordinateFields.some(item => item.value === track.coordinate_system)) coordinateFields.push({ value: track.coordinate_system, label: '历史值（' + track.coordinate_system + '）' });
        isDeleted = track.status === 0;
        const labelAbort = new AbortController();
        let labelBadges = null, placeCandidates = null, mapPreview = null;
        let detailLayout, workoutFields, shapeFields, patternPicker, patternValue, patternAdd, patternActions;
        function renderLabelBadges() {
          if (!labelBadges) return;
          labelBadges.replaceChildren();
          (track.labels || []).filter(label => label.value !== false && !['sport', 'shape', 'scenic_spot'].includes(label.type)).forEach(label => {
            const badge = document.createElement('span'); badge.className = 'badge bg-secondary-lt';
            badge.textContent = label.name || label.type; labelBadges.appendChild(badge);
          });
          labelBadges.hidden = !labelBadges.childElementCount;
        }
        function updateScenicSpot(data) {
          if (labelAbort.signal.aborted) return;
          track.primary_scenic_spot = data.primary_scenic_spot || null;
          trackPayload.scenic_spot_id = data.scenic_spot_id;
          const labels = (track.labels || []).filter(label => label.type !== 'scenic_spot');
          const scenic = (data.labels || []).filter(label => label.type === 'scenic_spot');
          const shapeIndex = labels.findIndex(label => label.type === 'shape');
          labels.splice(shapeIndex < 0 ? labels.length : shapeIndex, 0, ...scenic);
          track.labels = labels;
          renderLabelBadges();
          if (placeCandidates) placeCandidates.setPrimarySpot(track.primary_scenic_spot, data.scenic_spot_id, data.place_recognition_status);
          if (mapPreview) mapPreview.setScenicSpot(track.primary_scenic_spot, data.scenic_spot_id);
        }
        async function refreshScenicSpot() {
          const response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID), { signal: labelAbort.signal });
          const result = await response.json();
          if (!response.ok) throw new Error(result.error || '景点标签刷新失败');
          const data = result.data;
          updateScenicSpot({ primary_scenic_spot: data.track.primary_scenic_spot, labels: data.track.labels, scenic_spot_id: data.scenic_spot_id, place_recognition_status: data.place_recognition_status });
        }
        async function notifyScenicSpotChanged() {
          if (!labelAbort.signal.aborted && typeof options.onSaved === 'function') await options.onSaved();
        }
        mapPreview = window.AdminTrackMapPreview ? window.AdminTrackMapPreview.create(trackPayload.geo, {
          trackID: trackID, coordinateSystem: track.coordinate_system,
          scenicSpotID: trackPayload.scenic_spot_id, primaryScenicSpot: track.primary_scenic_spot,
          canAssignScenicSpot: !readOnly && !isDeleted && window.AdminAccess.can('scenic_spots', true) && typeof trackPayload.scenic_spot_id === 'string',
          onScenicSpotAssigned: async function (data) { updateScenicSpot(data); await notifyScenicSpotChanged(); },
          onScenicSpotRefresh: refreshScenicSpot,
          onNearbyResults: function (state) { if (placeCandidates) placeCandidates.showNearby(state); }
        }) : null;
        const assets = trackAssets(trackID, track, labelAbort.signal);
        function setupRouteDescription(view) {
          const input = view.form.elements.description;
          const oldGroup = input.parentNode;
          const group = document.createElement('div'); group.className = oldGroup.className + ' admin-track-description';
          oldGroup.replaceWith(group);
          input.setAttribute('aria-label', '路线说明');
          const heading = document.createElement('div'); heading.className = 'admin-track-description-heading';
          const title = document.createElement('span'); title.textContent = '路线说明'; heading.appendChild(title);
          const hint = document.createElement('p'); hint.className = 'text-secondary mb-0 mt-1';
          hint.textContent = readOnly || isDeleted ? '当前已保存的路线说明。' : 'AI 建议会先展示在下方，确认后可单独代入并保存。';
          hint.setAttribute('role', 'status'); hint.setAttribute('aria-live', 'polite');
          group.append(heading, input, hint);
          if (readOnly || isDeleted) return;
          const button = document.createElement('button'); button.type = 'button'; button.className = 'btn btn-sm btn-outline-primary';
          button.textContent = 'AI 联网填写'; button.setAttribute('aria-label', 'AI 联网填写'); heading.appendChild(button);
          const candidateArea = document.createElement('section'); candidateArea.className = 'admin-track-description-candidate'; candidateArea.hidden = true;
          const candidateTitle = document.createElement('strong'); candidateTitle.textContent = 'AI 建议说明';
          const candidateInput = document.createElement('textarea'); candidateInput.className = 'form-control'; candidateInput.rows = 3; candidateInput.maxLength = 4000;
          candidateInput.setAttribute('aria-label', 'AI 建议说明');
          const sources = document.createElement('div'); sources.className = 'admin-track-description-sources';
          const actions = document.createElement('div'); actions.className = 'admin-track-description-actions';
          const apply = document.createElement('button'); apply.type = 'button'; apply.className = 'btn btn-sm btn-primary';
          apply.textContent = '一键代入并保存'; apply.setAttribute('aria-label', '一键代入并保存');
          const note = document.createElement('span'); note.className = 'text-secondary'; note.textContent = '只保存路线说明，其他未保存的修改会保留。';
          actions.append(apply, note); candidateArea.append(candidateTitle, candidateInput, sources, actions); group.appendChild(candidateArea);
          let revision = 0, candidateVersion = '', candidateTaskID = '', generating = false, applying = false;
          view.form.addEventListener('input', function () { revision++; });
          view.form.addEventListener('change', function () { revision++; });
          function updateButtons() {
            button.disabled = generating || applying;
            button.textContent = generating ? '正在联网生成…' : 'AI 联网填写';
            apply.disabled = generating || applying || !candidateVersion || !candidateInput.value.trim();
            apply.textContent = applying ? '正在保存…' : '一键代入并保存';
          }
          candidateInput.addEventListener('input', updateButtons);
          button.addEventListener('click', async function (event) {
            event.preventDefault();
            if (generating || applying || view.signal.aborted) return;
            const originalRevision = revision;
            const originalSpot = trackPayload.scenic_spot_id;
            const originalForm = new URLSearchParams(new FormData(view.form)).toString();
            generating = true; updateButtons(); hint.textContent = '正在检索地点背景并结合轨迹资料生成建议…';
            try {
              const response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID) + '/description/generate', { method: 'POST', signal: view.signal });
              const result = await response.json();
              if (!response.ok) throw new Error(result.error || 'AI 联网填写失败');
              if (view.signal.aborted || !group.isConnected) return;
              if (revision !== originalRevision || trackPayload.scenic_spot_id !== originalSpot || new URLSearchParams(new FormData(view.form)).toString() !== originalForm) {
                hint.textContent = '生成期间资料有修改，本次建议未覆盖草稿。请保存资料后重新生成。';
                return;
              }
              const data = result.data || {};
              if (!String(data.description || '').trim() || !data.source_version) throw new Error('AI 未返回有效建议，原说明已保留');
              candidateInput.value = data.description; candidateVersion = data.source_version; candidateTaskID = data.task_id || '';
              candidateArea.hidden = false; sources.replaceChildren();
              hint.textContent = '建议已生成，原说明未变；可编辑建议，确认后点击“一键代入并保存”。';
              const sourceList = Array.isArray(data.sources) ? data.sources : [];
              if (sourceList.length) {
                const caption = document.createElement('span'); caption.textContent = '检索参考来源：'; sources.appendChild(caption);
                sourceList.forEach(function (source) {
                  let url;
                  try { url = new URL(source.url); } catch (_) { return; }
                  if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) return;
                  const link = document.createElement('a'); link.href = url.href; link.target = '_blank'; link.rel = 'noopener noreferrer';
                  link.textContent = source.title || url.hostname; sources.appendChild(link);
                });
              }
            } catch (error) {
              if (!view.signal.aborted && error.name !== 'AbortError') hint.textContent = error.message || 'AI 联网填写失败，原说明已保留';
            } finally {
              generating = false;
              if (!view.signal.aborted) updateButtons();
            }
          });
          apply.addEventListener('click', async function (event) {
            event.preventDefault();
            if (generating || applying || !candidateVersion || !candidateInput.value.trim() || view.signal.aborted) return;
            const description = candidateInput.value.trim();
            const original = input.value;
            applying = true; updateButtons(); hint.textContent = '正在单独保存路线说明…';
            let responseReceived = false;
            try {
              const response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID) + '/description', {
                method: 'PUT', headers: { 'Content-Type': 'application/json' }, signal: view.signal,
                body: JSON.stringify({ description: description, source_version: candidateVersion, task_id: candidateTaskID })
              });
              const result = await response.json(); responseReceived = true;
              if (!response.ok) throw new Error(result.error || '路线说明保存失败');
              if (!result.data || result.data.saved !== true) throw new Error('保存结果未确认，请重新打开轨迹核实');
              track.description = result.data.description; track.description_source = 'manual'; track.description_rule_version = 0;
              candidateVersion = '';
              if (view.signal.aborted || !group.isConnected) return;
              candidateArea.hidden = true;
              if (input.value === original) {
                input.value = track.description;
                input.dispatchEvent(new Event('input', { bubbles: true }));
                hint.textContent = '路线说明已代入并保存，其他未保存的修改保持原样。';
              } else {
                hint.textContent = 'AI 说明已保存；你刚编辑的说明草稿仍保留在输入框中，可另行保存。';
              }
              if (typeof options.onSaved === 'function') {
                try { await options.onSaved(); }
                catch (_) { if (!view.signal.aborted) hint.textContent += ' 列表刷新失败，请稍后刷新。'; }
              }
            } catch (error) {
              if (!view.signal.aborted && error.name !== 'AbortError') {
                hint.textContent = responseReceived ? error.message : '保存结果未确认，请重新打开轨迹核实；建议内容已保留。';
              }
            } finally {
              applying = false;
              if (!view.signal.aborted) updateButtons();
            }
          });
          updateButtons();
        }
        return {
          title: (readOnly ? '查看轨迹 ' : '编辑轨迹 ') + trackID,
          readOnly: readOnly,
          onReady: function (view) {
            view.form.prepend(detailLayout);
            detailLayout.classList.add('full');
            const editable = !readOnly && !isDeleted;
            const workout = view.form.elements.track_type;
            workout.className = 'form-select form-select-sm admin-track-sport-select'; workout.setAttribute('aria-label', '运动类型'); workout.disabled = !editable;
            Array.from(workout.parentNode.childNodes).forEach(node => { if (node.nodeType === Node.TEXT_NODE) node.remove(); });
            workoutFields.appendChild(workout.parentNode);
            const pattern = view.form.elements.shape_label, patternGroup = pattern.parentNode;
            patternAdd.hidden = !editable;
            pattern.setAttribute('aria-label', '图案标签'); pattern.readOnly = !editable; patternGroup.classList.add('admin-track-pattern-input');
            Array.from(patternGroup.childNodes).forEach(node => { if (node.nodeType === Node.TEXT_NODE) node.remove(); });
            const renderPattern = () => {
              const statuses = { waiting: 'AI等待查询', running: 'AI正在查询', not_found: 'AI没有查到' };
              patternValue.textContent = pattern.value.trim() || statuses[(trackPayload.label_generation || {}).shape_recognition_status] || '未设置'; patternValue.title = patternValue.textContent;
              patternValue.classList.toggle('is-empty', !pattern.value.trim());
            };
            pattern.addEventListener('input', renderPattern); pattern.addEventListener('change', renderPattern); renderPattern();
            pattern.addEventListener('keydown', event => {
              if (event.key === 'Enter' && !event.isComposing) { event.preventDefault(); patternPicker.open = false; patternPicker.querySelector('summary').focus(); }
            });
            if (editable) {
              const done = document.createElement('button'); done.type = 'button'; done.className = 'btn btn-sm btn-outline-primary'; done.textContent = '确定'; done.dataset.adminWrite = 'tracks';
              done.addEventListener('click', () => { patternPicker.open = false; patternPicker.querySelector('summary').focus(); });
              const clear = document.createElement('button'); clear.type = 'button'; clear.className = 'btn btn-sm btn-outline-secondary'; clear.textContent = '清除'; clear.dataset.adminWrite = 'tracks';
              clear.addEventListener('click', () => { pattern.value = ''; pattern.dispatchEvent(new Event('input', { bubbles: true })); pattern.dispatchEvent(new Event('change', { bubbles: true })); patternPicker.open = false; patternPicker.querySelector('summary').focus(); });
              const note = document.createElement('span'); note.className = 'text-secondary'; note.textContent = '保存后生效';
              patternActions.append(done, clear, note);
            }
            shapeFields.prepend(patternGroup);
            patternPicker.addEventListener('toggle', () => { if (patternPicker.open && editable) pattern.focus(); });
            view.form.addEventListener('toggle', event => {
              const picker = event.target;
              if (!picker.matches('.admin-track-label-picker') || !picker.open) return;
              view.form.querySelectorAll('.admin-track-label-picker[open]').forEach(other => { if (other !== picker) other.open = false; });
            }, true);
            view.form.addEventListener('keydown', event => {
              const picker = event.target.closest('.admin-track-label-picker[open]');
              if (event.key === 'Escape' && picker) { event.preventDefault(); event.stopPropagation(); picker.open = false; picker.querySelector('summary').focus(); }
            }, true);
            document.addEventListener('pointerdown', event => {
              view.form.querySelectorAll('.admin-track-label-picker[open]').forEach(picker => { if (!picker.contains(event.target)) picker.open = false; });
            }, { signal: view.signal });
            if (mapPreview) mapPreview.setCoordinateControl(view.form.elements.coordinate_system.parentNode);
            const layout = document.createElement('div'); layout.className = 'admin-track-content-grid full';
            const main = document.createElement('div'); main.className = 'admin-track-main';
            layout.append(main, assets); view.form.appendChild(layout);
            const basic = document.createElement('section'); basic.className = 'admin-track-basic full';
            const basicTitle = document.createElement('h3'); basicTitle.textContent = '基本信息';
            const basicFields = document.createElement('div'); basicFields.className = 'admin-track-basic-fields';
            ['title', 'user_name', 'city_code', 'locate_addr', 'source_tag', 'status', 'session_id'].forEach(name => {
              const group = view.form.elements[name].parentNode;
              if (name === 'title' || name === 'locate_addr') group.classList.add('admin-track-field-wide');
              basicFields.appendChild(group);
            });
            basic.append(basicTitle, basicFields);
            main.appendChild(basic);
            const metrics = trackMetrics(track); main.appendChild(metrics);
            setupRouteDescription(view);
            main.appendChild(view.form.elements.description.parentNode);
            if (assets.loadMetadata) assets.loadMetadata();
            if (Array.isArray(options.cities)) return;
            const select = view.form.elements.city_code;
            const hint = document.createElement('span'); hint.className = 'text-secondary d-block mt-1';
            select.parentNode.appendChild(hint);
            async function loadCities() {
              hint.replaceChildren(); hint.textContent = '正在加载城市选项…';
              try {
                const response = await fetch('/admin/api/cities', { signal: view.signal });
                if (!response.ok) throw new Error('城市选项加载失败');
                const items = ((await response.json()).data) || [];
                if (view.signal.aborted) return;
                const selected = select.value;
                items.forEach(city => { cityNames[city.code] = city.name; });
                if (selected && !items.some(city => city.code === selected)) items.push({ code: selected, name: cityNames[selected] || trackPayload.city_name || selected });
                select.replaceChildren(new Option('未设置', ''));
                items.forEach(city => select.add(new Option(city.name, city.code)));
                select.value = selected; hint.remove();
              } catch (error) {
                if (view.signal.aborted) return;
                hint.textContent = '城市选项加载失败，当前城市已保留。';
                const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-sm btn-link'; retry.textContent = '重试';
                retry.addEventListener('click', loadCities); hint.appendChild(retry);
              }
            }
            loadCities();
          },
          onClosed: function () { labelAbort.abort(); if (mapPreview) mapPreview.dispose(); if (placeCandidates) placeCandidates.dispose(); },
          waitForHidden: true,
          confirmText: isDeleted ? '彻底删除' : '保存修改',
          cancelText: readOnly ? '关闭' : (isDeleted ? '恢复' : '删除'),
          cancelDanger: !readOnly && !isDeleted,
          onCancel: readOnly ? null : isDeleted ? async function () {
            const confirmed = await window.AdminUI.confirm({ title: '恢复轨迹', message: '确认恢复轨迹 ' + trackID + '？恢复后状态为公开，并会重新排队地图索引；删除时清理的收藏与路线组成员关系不会恢复。', confirmText: '恢复轨迹' });
            if (!confirmed) return false;
            const response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID) + '/restore', { method: 'POST' });
            if (!response.ok) {
              let message = '恢复失败: ' + response.status;
              try { const result = await response.json(); if (result && result.error) message += ' ' + result.error; } catch (_) {}
              window.AdminUI.alert(message, 'danger');
              return false;
            }
            window.AdminUI.alert('轨迹已恢复为公开状态', 'success');
            return 'restored';
          } : async function () {
            const confirmed = await window.AdminUI.confirm({ title: '删除轨迹', message: '确认删除轨迹 ' + trackID + '？删除后标记为下架（数据保留，可在已下架筛选中恢复），并清理收藏和地图索引。', confirmText: '删除轨迹', danger: true });
            if (!confirmed) return false;
            const response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID), { method: 'DELETE' });
            if (!response.ok) {
              let message = '删除失败: ' + response.status;
              try { const result = await response.json(); if (result && result.error) message += ' ' + result.error; } catch (_) {}
              window.AdminUI.alert(message, 'danger');
              return false;
            }
            window.AdminUI.alert('轨迹已下架', 'success');
            return 'deleted';
          },
          wide: true,
          className: 'admin-track-editor',
          afterFields: assets,
          beforeFields: (function () {
            const wrapper = document.createElement('div');
            wrapper.className = 'admin-track-detail-side'; detailLayout = wrapper;
            const panel = document.createElement('section');
            panel.className = 'card';
            const content = document.createElement('div');
            content.className = 'card-body';
            labelBadges = document.createElement('div'); labelBadges.className = 'd-flex flex-wrap gap-2 mb-2';
            renderLabelBadges();
            content.appendChild(labelBadges);
            const workoutSection = document.createElement('section'); workoutSection.className = 'admin-track-workout-row';
            const workoutTitle = document.createElement('h4'); workoutTitle.className = 'admin-track-label-caption'; workoutTitle.textContent = '运动类型';
            workoutFields = document.createElement('div'); workoutSection.append(workoutTitle, workoutFields);
            const shapeSection = document.createElement('details'); shapeSection.className = 'admin-track-label-picker'; patternPicker = shapeSection;
            const shapeHeading = document.createElement('summary'); shapeHeading.className = 'admin-track-label-summary';
            const shapeTitle = document.createElement('span'); shapeTitle.className = 'admin-track-label-caption'; shapeTitle.textContent = '图案';
            const shapeValues = document.createElement('span'); shapeValues.className = 'admin-track-label-values';
            patternValue = document.createElement('span'); patternValue.className = 'admin-track-label-current';
            patternAdd = document.createElement('span'); patternAdd.className = 'admin-track-label-add'; patternAdd.textContent = '+'; patternAdd.setAttribute('aria-hidden', 'true');
            shapeHeading.setAttribute('aria-label', '查看或设置图案'); shapeValues.append(patternValue, patternAdd); shapeHeading.append(shapeTitle, shapeValues);
            shapeFields = document.createElement('div'); shapeFields.className = 'admin-track-label-options';
            patternActions = document.createElement('div'); patternActions.className = 'd-flex flex-wrap gap-2'; shapeFields.appendChild(patternActions);
            shapeSection.append(shapeHeading, shapeFields);
            content.append(workoutSection, shapeSection);
            panel.appendChild(content); wrapper.appendChild(panel);
            placeCandidates = window.AdminTrackPlaceCandidates.create(trackID, { readOnly: readOnly || isDeleted,
              onChanged: async function () { await refreshScenicSpot(); await notifyScenicSpotChanged(); },
              onAssigned: async function (data) { updateScenicSpot(data); await notifyScenicSpotChanged(); },
              scenicSpotID: trackPayload.scenic_spot_id, track: track, recognitionStatus: trackPayload.place_recognition_status });
            content.appendChild(placeCandidates);
            if (mapPreview) wrapper.appendChild(mapPreview);
            return wrapper;
          })(),
          fields: [
            { name: 'user_name', label: '归属用户名', value: trackPayload.user_name || '未知用户', readOnly: true },
            { name: 'session_id', label: '同行会话 ID', value: track.session_id },
            { name: 'shape_label', label: '图案标签', placeholder: '填写图案名称，留空表示无', value: track.shape_label || '', maxLength: 32 },
            { name: 'title', label: '轨迹标题', value: track.title, maxLength: 128 },
            { name: 'city_code', label: '城市', type: 'select', value: track.city_code, options: cityFields },
            { name: 'locate_addr', label: '位置描述', value: track.locate_addr, maxLength: 512 },
            { name: 'track_type', label: '运动类型', type: 'select', value: track.track_type, options: typeFields },
            { name: 'source_tag', label: '来源标签', type: 'select', value: track.source_tag, options: sourceFields },
            { name: 'coordinate_system', label: '坐标系', type: 'select', value: track.coordinate_system, options: coordinateFields, onChange: function (value) { if (mapPreview) mapPreview.setCoordinateSystem(value); } },
            { name: 'description', label: '路线说明', type: 'textarea', value: track.description, maxLength: 4000, full: true },
            { name: 'status', label: '可见状态', type: 'select', value: track.status, options: [{ value: 1, label: '正常' }, { value: 2, label: '私密' }] }
          ]
        };
      }
    });
    if (!values) return false;
    if (values._action === 'deleted') {
      if (typeof options.onDeleted === 'function') await options.onDeleted();
      return false;
    }
    if (values._action === 'restored') {
      if (typeof options.onSaved === 'function') await options.onSaved();
      return false;
    }
    if (isDeleted) {
      // 已下架轨迹的主按钮是「彻底删除」：双重确认后物理删除，不可恢复。
      const first = await window.AdminUI.confirm({ title: '彻底删除轨迹', message: '彻底删除 ' + trackID + ' 将物理删除数据库记录与关联数据（收藏、评论、点赞、索引、景点关联、投稿），并清理 OSS 原始文件与预览图，不可恢复。确认继续？', confirmText: '继续', danger: true });
      if (!first) return false;
      const second = await window.AdminUI.confirm({ title: '再次确认', message: '这是最后确认：' + trackID + ' 删除后服务端不再留存任何数据。', confirmText: '彻底删除', danger: true });
      if (!second) return false;
      const purgeResponse = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID) + '/purge', { method: 'POST' });
      if (!purgeResponse.ok) {
        let message = '彻底删除失败: ' + purgeResponse.status;
        try { const result = await purgeResponse.json(); if (result && result.error) message += ' ' + result.error; } catch (_) {}
        window.AdminUI.alert(message, 'danger');
        return false;
      }
      try {
        const result = await purgeResponse.json();
        const warning = result && result.data && result.data.oss_warning;
        window.AdminUI.alert(warning || '轨迹已彻底删除', warning ? 'warning' : 'success');
      } catch (_) {
        window.AdminUI.alert('轨迹已彻底删除', 'success');
      }
      if (typeof options.onDeleted === 'function') await options.onDeleted();
      return false;
    }
    const body = {
      source_feedback_id: options.sourceFeedbackID || undefined,
      session_id: values.session_id, title: values.title, city_code: values.city_code,
      shape_label: values.shape_label !== (track.shape_label || '') ? values.shape_label : undefined,
      locate_addr: values.locate_addr, track_type: values.track_type, source_tag: values.source_tag,
      coordinate_system: values.coordinate_system,
      description: values.description !== (track.description || '') ? values.description : undefined,
      status: Number(values.status)
    };
    try {
      const response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      if (!response.ok) {
        let message = '保存失败: ' + response.status;
        try { const result = await response.json(); if (result && result.error) message += ' ' + result.error; } catch (_) {}
        window.AdminUI.alert(message, 'danger');
        return false;
      }
      const saved = await response.json();
      if (saved.learning_warning || saved.correction_warning) window.AdminUI.alert([saved.learning_warning, saved.correction_warning].filter(Boolean).join('；'), 'warning');
      else window.AdminUI.alert('轨迹已保存', 'success');
      if (typeof options.onSaved === 'function') await options.onSaved();
      return true;
    } catch (error) {
      window.AdminUI.alert('保存失败: ' + (error.message || error), 'danger');
      return false;
    }
  }

  window.AdminTrackEditor = {
    open: function (trackID, options) {
      return window.AdminUI.runOnce('track-editor', function () { return openTrackEditor(trackID, options); });
    }
  };
})();
