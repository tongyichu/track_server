(function () {
  'use strict';

  function trackAssets(trackID, track) {
    const section = document.createElement('section');
    section.className = 'admin-track-assets';
    const fileRow = document.createElement('div');
    fileRow.className = 'admin-track-file';
    let trackType = track.track_type || '';
    let noMapImage, noMapStage;
    const assetURL = asset => '/admin/api/tracks/' + encodeURIComponent(trackID) + '/assets/' + asset +
      (asset === 'no_map' ? '?track_type=' + encodeURIComponent(trackType) : '');
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
      fileRow.append(download, label);
    } else {
      const missing = document.createElement('span');
      missing.className = 'text-secondary';
      missing.textContent = '缺失';
      fileRow.append(label, missing);
    }
    section.appendChild(fileRow);
    const grid = document.createElement('div');
    grid.className = 'admin-track-preview-grid';
    const previews = track.preview_images || {};
    [
      ['square_day', '方形日间', previews.square_day_url],
      ['square_night', '方形夜间', previews.square_night_url],
      ['landscape_day', '横版日间', previews.landscape_day_url],
      ['landscape_night', '横版夜间', previews.landscape_night_url],
      ['legacy', '旧版截图', track.track_screenshot_url],
      ['no_map', '无地图截图', track.track_no_map_bg_screenshot_url]
    ].forEach(function (asset) {
      const figure = document.createElement('figure');
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
        if (asset[0] === 'no_map') { noMapImage = image; noMapStage = stage; }
      } else {
        stage.textContent = '无';
      }
      figure.append(caption, stage);
      grid.appendChild(figure);
    });
    section.appendChild(grid);
    section.setTrackType = function (value) {
      trackType = value;
      if (noMapImage) {
        noMapStage.replaceChildren(noMapImage);
        noMapImage.src = assetURL('no_map');
      }
    };
    return section;
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
        function timeValue(value) {
          if (!value) return '';
          const date = new Date(value);
          if (Number.isNaN(date.getTime())) return '';
          const pad = number => String(number).padStart(2, '0');
          return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + 'T' + pad(date.getHours()) + ':' + pad(date.getMinutes());
        }
        isDeleted = track.status === 0;
        const labelAbort = new AbortController();
        let labelBadges = null, placeCandidates = null, mapPreview = null;
        function renderLabelBadges() {
          if (!labelBadges) return;
          labelBadges.replaceChildren();
          (track.labels || []).filter(label => label.value !== false).forEach(label => {
            const badge = document.createElement('span'); badge.className = 'badge bg-secondary-lt';
            badge.textContent = label.name || label.type; labelBadges.appendChild(badge);
          });
          if (!labelBadges.childElementCount) labelBadges.textContent = '暂无标签';
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
          if (placeCandidates) placeCandidates.setPrimarySpot(track.primary_scenic_spot);
          if (mapPreview) mapPreview.setScenicSpot(track.primary_scenic_spot, data.scenic_spot_id);
        }
        async function refreshScenicSpot() {
          const response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID), { signal: labelAbort.signal });
          const result = await response.json();
          if (!response.ok) throw new Error(result.error || '景点标签刷新失败');
          const data = result.data;
          updateScenicSpot({ primary_scenic_spot: data.track.primary_scenic_spot, labels: data.track.labels, scenic_spot_id: data.scenic_spot_id });
        }
        async function notifyScenicSpotChanged() {
          if (!labelAbort.signal.aborted && typeof options.onSaved === 'function') await options.onSaved();
        }
        mapPreview = window.AdminTrackMapPreview ? window.AdminTrackMapPreview.create(trackPayload.geo, {
          trackID: trackID, coordinateSystem: track.coordinate_system,
          scenicSpotID: trackPayload.scenic_spot_id, primaryScenicSpot: track.primary_scenic_spot,
          canAssignScenicSpot: !readOnly && !isDeleted && window.AdminAccess.can('scenic_spots', true) && typeof trackPayload.scenic_spot_id === 'string',
          onScenicSpotAssigned: async function (data) { updateScenicSpot(data); await notifyScenicSpotChanged(); },
          onScenicSpotRefresh: refreshScenicSpot
        }) : null;
        const assets = trackAssets(trackID, track);
        return {
          title: (readOnly ? '查看轨迹 ' : '编辑轨迹 ') + trackID,
          readOnly: readOnly,
          onReady: function (view) {
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
            wrapper.className = 'admin-track-detail-side';
            const panel = document.createElement('section');
            panel.className = 'card';
            const content = document.createElement('div');
            content.className = 'card-body';
            const heading = document.createElement('h3');
            heading.className = 'card-title'; heading.textContent = '轨迹标签'; content.appendChild(heading);
            labelBadges = document.createElement('div'); labelBadges.className = 'd-flex flex-wrap gap-2 mb-2';
            renderLabelBadges();
            content.appendChild(labelBadges);
            const states = { waiting: '等待自动生成', running: '正在生成', ready: '生成完成，等待写入', complete: '自动生成已完成（无明确形象时不打标）', retry: '等待重试', failed: '自动生成失败', not_queued: '未进入自动任务', unconfigured: 'AI 未配置' };
            const status = document.createElement('p'); status.className = 'text-secondary mb-2';
            const generation = trackPayload.label_generation || {};
            status.textContent = (states[generation.status] || '状态暂不可用') + (generation.attempts ? '，已尝试 ' + generation.attempts + ' 次' : '') + (generation.reason ? '：' + generation.reason : '');
            content.appendChild(status);
            const diagnosis = document.createElement('div'); diagnosis.className = 'mb-2'; content.appendChild(diagnosis);
            function showDiagnosis(value) {
              diagnosis.replaceChildren();
              if (!value) return;
              const summary = document.createElement('p'); summary.className = 'text-secondary mb-1';
              const outcome = { recognized: '识别到候选形象', ambiguous: '形象存在歧义', no_candidate: '未找到明确候选' };
              const suitability = { appropriate: '适宜展示', inappropriate: '不适宜展示', undetermined: '适宜性未确定' };
              summary.textContent = (outcome[value.recognition_status] || '') + '；' + (suitability[value.appropriateness] || '') + '。' + (value.decision_summary || '');
              diagnosis.appendChild(summary);
              const details = document.createElement('details');
              const title = document.createElement('summary'); title.textContent = '查看分类检查与候选依据'; details.appendChild(title);
              const list = document.createElement('ul'); list.className = 'mt-2';
              (value.category_checks || []).forEach(function (check) {
                const item = document.createElement('li'); item.textContent = check.category + (check.candidate ? '（' + check.candidate + '）' : '') + '：' + check.visual_basis; list.appendChild(item);
              });
              (value.candidates || []).forEach(function (candidate) {
                const item = document.createElement('li'); item.textContent = '候选 ' + candidate.label + '：' + candidate.support + (candidate.conflict ? '；限制：' + candidate.conflict : ''); list.appendChild(item);
              });
              details.appendChild(list); diagnosis.appendChild(details);
            }
            showDiagnosis(generation.shape_preview && generation.shape_preview.result);
            if (generation.learning) {
              const learning = generation.learning;
              const names = { pending: '等待复核', running: '正在复核', retry: '等待重试', reviewed: '已复核，等待保存经验', learned: '已纳入识别经验', disputed: '图像依据不足，未采纳为经验', stale: '资料已变化，样例已失效', failed: '复核失败', unavailable: '状态读取失败' };
              const note = document.createElement('p'); note.className = 'text-secondary mb-2';
              note.textContent = '人工标注学习：' + (names[learning.status] || learning.status) + (learning.review ? '。依据：' + learning.review.evidence + '。经验：' + learning.review.lesson : '');
              content.appendChild(note);
            }
            const help = document.createElement('p'); help.className = 'text-secondary mb-2';
            help.textContent = '形象可在下方人工编辑或用 AI 生成。精品随投稿审核生效；运动类型在表单修改，景点可在地图点选或确认候选后立即关联。AI 基于已保存资料和一张方图，生成后需保存才生效。';
            content.appendChild(help);
            if (!readOnly && !isDeleted) {
              const button = document.createElement('button'); button.type = 'button'; button.className = 'btn btn-outline-primary'; button.textContent = 'AI 生成形象';
              button.addEventListener('click', async function () {
                const modal = wrapper.closest('.modal');
                const input = modal && modal.querySelector('[name="shape_label"]');
                if (!input) return;
                const original = input.value;
                showDiagnosis(null);
                button.disabled = true; status.textContent = '正在分析已保存的轨迹和方形预览图…';
                try {
                  const response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID) + '/labels/generate', { method: 'POST', signal: labelAbort.signal });
                  const result = await response.json();
                  if (!response.ok) throw new Error(result.error || 'AI 生成失败');
                  if (!wrapper.isConnected) return;
                  showDiagnosis(result.data.diagnosis);
                  if (input.value !== original) { status.textContent = '你已修改形象输入，本次结果未覆盖，请重新生成或保存人工内容。'; return; }
                  input.value = result.data.shape_label || '';
                  input.dispatchEvent(new Event('change', { bubbles: true }));
                  status.textContent = input.value ? '建议形象：' + input.value + '。点击保存修改后生效。' : '没有识别到明确且适宜的形象，建议留空。点击保存修改后生效。';
                } catch (error) {
                  if (error.name !== 'AbortError') { status.textContent = error.message; window.AdminUI.alert(error.message, 'danger'); }
                } finally { button.disabled = false; }
              });
              content.appendChild(button);
            }
            panel.appendChild(content); wrapper.appendChild(panel);
            placeCandidates = window.AdminTrackPlaceCandidates.create(trackID, { readOnly: readOnly || isDeleted,
              onChanged: async function () { await refreshScenicSpot(); await notifyScenicSpotChanged(); }, cityNames: cityNames, track: track });
            wrapper.appendChild(placeCandidates);
            if (mapPreview) wrapper.appendChild(mapPreview);
            return wrapper;
          })(),
          fields: [
            { name: 'user_name', label: '归属用户名', value: trackPayload.user_name || '未知用户', readOnly: true },
            { name: 'session_id', label: '同行会话 ID', value: track.session_id },
            { name: 'shape_label', label: '形象标签', placeholder: '明确且适宜的形象，留空表示无', value: track.shape_label || '', maxLength: 32 },
            { name: 'title', label: '轨迹标题', value: track.title, maxLength: 128 },
            { name: 'city_code', label: '城市', type: 'select', value: track.city_code, options: cityFields },
            { name: 'locate_addr', label: '位置描述', value: track.locate_addr, maxLength: 512 },
            { name: 'track_type', label: '轨迹类型', type: 'select', value: track.track_type, options: typeFields, onChange: assets.setTrackType },
            { name: 'source_tag', label: '来源标签', type: 'select', value: track.source_tag, options: sourceFields },
            { name: 'coordinate_system', label: '坐标系', type: 'select', value: track.coordinate_system, options: coordinateFields, onChange: function (value) { if (mapPreview) mapPreview.setCoordinateSystem(value); } },
            { name: 'description', label: '轨迹描述', type: 'textarea', value: track.description, maxLength: 4000, full: true },
            { name: 'start_time', label: '开始时间', type: 'datetime-local', value: timeValue(track.start_time), required: true },
            { name: 'end_time', label: '结束时间', type: 'datetime-local', value: timeValue(track.end_time) },
            { name: 'distance', label: '距离（米）', type: 'number', value: track.distance, min: 0, step: 'any' },
            { name: 'duration', label: '时长（秒）', type: 'number', value: track.duration, min: 0 },
            { name: 'calories_burned', label: '消耗热量（千卡）', type: 'number', value: track.calories_burned, min: 0, step: 'any' },
            { name: 'elevation_gain', label: '累计爬升（米）', type: 'number', value: track.elevation_gain },
            { name: 'avg_speed_kmh', label: '平均速度（公里/小时）', type: 'number', value: track.avg_speed_kmh, min: 0, step: 'any' },
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
      coordinate_system: values.coordinate_system, description: values.description, start_time: values.start_time,
      end_time: values.end_time, distance: Number(values.distance || 0), duration: Number(values.duration || 0),
      calories_burned: Number(values.calories_burned || 0), elevation_gain: Number(values.elevation_gain || 0),
      avg_speed_kmh: Number(values.avg_speed_kmh || 0), status: Number(values.status)
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
