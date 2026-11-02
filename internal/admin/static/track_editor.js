(function () {
  'use strict';

  async function openTrackEditor(trackID, options) {
    options = options || {};
    if (!trackID) return false;
    if (!window.AdminAccess.can('tracks', false)) { window.AdminUI.alert('没有轨迹管理的查看权限', 'warning'); return false; }
    const readOnly = !window.AdminAccess.can('tracks', true);
    const [trackResponse, citiesResponse] = await Promise.all([
      fetch('/admin/api/tracks/' + encodeURIComponent(trackID)),
      fetch('/admin/api/cities')
    ]);
    if (!trackResponse.ok) {
      window.AdminUI.alert(trackResponse.status === 404 ? '该轨迹已删除或不存在' : '读取轨迹详情失败: ' + trackResponse.status, 'danger');
      return false;
    }
    if (!citiesResponse.ok) throw new Error('城市列表加载失败: ' + citiesResponse.status);
    const trackPayload = ((await trackResponse.json()).data) || {};
    const track = trackPayload.track || trackPayload;
    const cities = ((await citiesResponse.json()).data) || [];
    const cityNames = Object.create(null);
    cities.forEach(city => { cityNames[city.code] = city.name; });
    if (track.city_code && !cityNames[track.city_code]) cities.push({ code: track.city_code, name: '未知城市 (' + track.city_code + ')' });
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
    const preview = track.preview_images || {};
    const isDeleted = track.status === 0;
    const mapPreview = window.AdminTrackMapPreview ? window.AdminTrackMapPreview.create(trackPayload.geo, { trackID: trackID, coordinateSystem: track.coordinate_system }) : null;
    const values = await window.AdminUI.form({
      title: (readOnly ? '查看轨迹 ' : '编辑轨迹 ') + trackID,
      readOnly: readOnly,
      onClosed: function () { if (mapPreview) mapPreview.dispose(); },
      waitForHidden: !!options.waitForHidden,
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
      beforeFields: (function () {
        const wrapper = document.createElement('div');
        wrapper.className = 'admin-track-detail-side';
        wrapper.appendChild(window.AdminTrackPlaceCandidates.create(trackID, { readOnly: readOnly, onChanged: options.onSaved, cityNames: cityNames, track: track }));
        if (mapPreview) wrapper.appendChild(mapPreview);
        return wrapper;
      })(),
      fields: [
        { name: 'user_id', label: '用户 ID（不可修改）', value: trackPayload.user_id_text || '', readOnly: true },
        { name: 'session_id', label: '同行会话 ID', value: track.session_id },
        { name: 'title', label: '轨迹标题', value: track.title, maxLength: 128 },
        { name: 'city_code', label: '城市', type: 'select', value: track.city_code, options: cityFields },
        { name: 'locate_addr', label: '位置描述', value: track.locate_addr, maxLength: 512 },
        { name: 'track_type', label: '轨迹类型', type: 'select', value: track.track_type, options: typeFields },
        { name: 'source_tag', label: '来源标签', type: 'select', value: track.source_tag, options: sourceFields },
        { name: 'coordinate_system', label: '坐标系', type: 'select', value: track.coordinate_system, options: coordinateFields, onChange: function (value) { if (mapPreview) mapPreview.setCoordinateSystem(value); } },
        { name: 'description', label: '轨迹描述', type: 'textarea', value: track.description, maxLength: 4000, full: true },
        { name: 'description_source', label: '描述来源', value: track.description_source, maxLength: 64 },
        { name: 'description_rule_version', label: '描述规则版本', type: 'number', value: track.description_rule_version, min: 0 },
        { name: 'start_time', label: '开始时间', type: 'datetime-local', value: timeValue(track.start_time), required: true },
        { name: 'end_time', label: '结束时间', type: 'datetime-local', value: timeValue(track.end_time) },
        { name: 'distance', label: '距离（米）', type: 'number', value: track.distance, min: 0, step: 'any' },
        { name: 'duration', label: '时长（秒）', type: 'number', value: track.duration, min: 0 },
        { name: 'calories_burned', label: '消耗热量（千卡）', type: 'number', value: track.calories_burned, min: 0, step: 'any' },
        { name: 'elevation_gain', label: '累计爬升（米）', type: 'number', value: track.elevation_gain },
        { name: 'avg_speed_kmh', label: '平均速度（公里/小时）', type: 'number', value: track.avg_speed_kmh, min: 0, step: 'any' },
        { name: 'is_running', label: '轨迹进行状态', type: 'select', value: track.is_running, options: [{ value: 'false', label: '已结束' }, { value: 'true', label: '进行中' }] },
        { name: 'status', label: '可见状态', type: 'select', value: track.status, options: [{ value: 1, label: '正常' }, { value: 2, label: '私密' }] },
        { name: 'raw_track_url', label: '原始轨迹文件 URL', value: track.raw_track_url, maxLength: 255, full: true },
        { name: 'track_screenshot_url', label: '旧版截图 URL', value: track.track_screenshot_url, maxLength: 255, full: true },
        { name: 'track_no_map_bg_screenshot_url', label: '无地图截图 URL', value: track.track_no_map_bg_screenshot_url, maxLength: 255, full: true },
        { name: 'preview_square_day', label: '方形日间预览图 URL', value: preview.square_day_url, maxLength: 2048, full: true },
        { name: 'preview_square_night', label: '方形夜间预览图 URL', value: preview.square_night_url, maxLength: 2048, full: true },
        { name: 'preview_landscape_day', label: '横版日间 4:3 预览图 URL', value: preview.landscape_day_url, maxLength: 2048, full: true },
        { name: 'preview_landscape_night', label: '横版夜间 4:3 预览图 URL', value: preview.landscape_night_url, maxLength: 2048, full: true }
      ]
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
      locate_addr: values.locate_addr, track_type: values.track_type, source_tag: values.source_tag,
      coordinate_system: values.coordinate_system, description: values.description, description_source: values.description_source,
      description_rule_version: Number(values.description_rule_version || 0), start_time: values.start_time,
      end_time: values.end_time, distance: Number(values.distance || 0), duration: Number(values.duration || 0),
      calories_burned: Number(values.calories_burned || 0), elevation_gain: Number(values.elevation_gain || 0),
      avg_speed_kmh: Number(values.avg_speed_kmh || 0), is_running: values.is_running === 'true', status: Number(values.status),
      raw_track_url: values.raw_track_url, track_screenshot_url: values.track_screenshot_url,
      track_no_map_bg_screenshot_url: values.track_no_map_bg_screenshot_url,
      preview_images: { square_day_url: values.preview_square_day, square_night_url: values.preview_square_night, landscape_day_url: values.preview_landscape_day, landscape_night_url: values.preview_landscape_night }
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
      if (saved.correction_warning) window.AdminUI.alert(saved.correction_warning, 'warning');
      else window.AdminUI.alert('轨迹已保存', 'success');
      if (typeof options.onSaved === 'function') await options.onSaved();
      return true;
    } catch (error) {
      window.AdminUI.alert('保存失败: ' + (error.message || error), 'danger');
      return false;
    }
  }

  window.AdminTrackEditor = { open: openTrackEditor };
})();
