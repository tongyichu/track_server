(function () {
  'use strict';

  async function openTrackEditor(trackID, options) {
    options = options || {};
    if (!trackID) return false;
    const [trackResponse, citiesResponse] = await Promise.all([
      fetch('/admin/api/tracks/' + encodeURIComponent(trackID)),
      fetch('/admin/api/cities')
    ]);
    if (!trackResponse.ok) {
      window.AdminUI.alert(trackResponse.status === 404 ? '该轨迹已删除或不存在' : '读取轨迹详情失败: ' + trackResponse.status, 'danger');
      return false;
    }
    if (!citiesResponse.ok) throw new Error('城市列表加载失败: ' + citiesResponse.status);
    const track = ((await trackResponse.json()).data) || {};
    const cities = ((await citiesResponse.json()).data) || [];
    const cityNames = Object.create(null);
    cities.forEach(city => { cityNames[city.code] = city.name; });
    if (track.city_code && !cityNames[track.city_code]) cities.push({ code: track.city_code, name: '未知城市 (' + track.city_code + ')' });
    const cityFields = [{ value: '', label: '未设置' }].concat(cities.map(city => ({ value: city.code, label: city.name })));
    const typeNames = { hiking: '徒步', running: '跑步', climbing: '爬山', riding: '骑行', driving: '自驾' };
    const typeFields = Object.keys(typeNames).map(value => ({ value, label: typeNames[value] }));
    if (track.track_type && !typeNames[track.track_type]) typeFields.push({ value: track.track_type, label: track.track_type });
    function timeValue(value) {
      if (!value) return '';
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return '';
      const pad = number => String(number).padStart(2, '0');
      return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + 'T' + pad(date.getHours()) + ':' + pad(date.getMinutes());
    }
    const preview = track.preview_images || {};
    const values = await window.AdminUI.form({
      title: '编辑轨迹 ' + trackID,
      confirmText: '保存修改',
      cancelText: '删除',
      cancelDanger: true,
      onCancel: async function () {
        const confirmed = await window.AdminUI.confirm({ title: '删除轨迹', message: '确认删除轨迹 ' + trackID + '？删除后会标记为已删除，并清理收藏和地图索引。', confirmText: '删除轨迹', danger: true });
        if (!confirmed) return false;
        const response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID), { method: 'DELETE' });
        if (!response.ok) {
          let message = '删除失败: ' + response.status;
          try { const result = await response.json(); if (result && result.error) message += ' ' + result.error; } catch (_) {}
          window.AdminUI.alert(message, 'danger');
          return false;
        }
        window.AdminUI.alert('轨迹已删除', 'success');
        return 'deleted';
      },
      wide: true,
      fields: [
        { name: 'user_id', label: '用户 ID', type: 'number', value: track.user_id, min: 0 },
        { name: 'session_id', label: '同行会话 ID', value: track.session_id },
        { name: 'title', label: '轨迹标题', value: track.title, maxLength: 128 },
        { name: 'city_code', label: '城市', type: 'select', value: track.city_code, options: cityFields },
        { name: 'locate_addr', label: '位置描述', value: track.locate_addr, maxLength: 512 },
        { name: 'track_type', label: '轨迹类型', type: 'select', value: track.track_type, options: typeFields },
        { name: 'source_tag', label: '来源标签', value: track.source_tag, maxLength: 128 },
        { name: 'coordinate_system', label: '坐标系', value: track.coordinate_system, maxLength: 32 },
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
    const body = {
      user_id: Number(values.user_id), session_id: values.session_id, title: values.title, city_code: values.city_code,
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
      window.AdminUI.alert('轨迹已保存', 'success');
      if (typeof options.onSaved === 'function') await options.onSaved();
      return true;
    } catch (error) {
      window.AdminUI.alert('保存失败: ' + (error.message || error), 'danger');
      return false;
    }
  }

  window.AdminTrackEditor = { open: openTrackEditor };
})();
