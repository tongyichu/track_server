(function () {
  'use strict';
  var controller, generation = 0;
  var names = {
    'android.permission.ACCESS_FINE_LOCATION': '精确定位',
    'android.permission.ACCESS_COARSE_LOCATION': '大致定位',
    'android.permission.ACCESS_BACKGROUND_LOCATION': '后台定位',
    'android.permission.CAMERA': '相机',
    'android.permission.RECORD_AUDIO': '麦克风',
    'android.permission.ACTIVITY_RECOGNITION': '身体活动识别',
    'android.permission.POST_NOTIFICATIONS': '通知授权',
    'android.permission.READ_MEDIA_IMAGES': '读取照片',
    'android.permission.READ_MEDIA_VIDEO': '读取视频',
    'android.permission.READ_MEDIA_VISUAL_USER_SELECTED': '访问选定照片和视频',
    'android.permission.READ_EXTERNAL_STORAGE': '读取存储',
    'android.permission.WRITE_EXTERNAL_STORAGE': '写入存储',
    'android.permission.REQUEST_INSTALL_PACKAGES': '安装未知应用',
    'app.notifications': '应用通知开关',
    'app.location_service': '系统定位开关',
    'app.battery_optimization_exemption': '后台运行豁免',
    'app.notification_channels': '通知渠道'
  };
  var groups = { runtime_permission: '应用权限', special_permission: '特殊权限', system_setting: '系统设置', notification_channel: '通知渠道', normal_permission: '系统自动授予的权限', other_permission: '其他权限' };
  function el(tag, text, cls) {
    var node = document.createElement(tag);
    if (text != null) node.textContent = text;
    if (cls) node.className = cls;
    return node;
  }
  function time(value) {
    return AdminTime.dateTime(value, '未知');
  }
  function badge(item) {
    var color = 'secondary', state = '未知';
    if (item.state === 'granted' || item.state === 'enabled') { color = 'success'; state = '已开启'; }
    else if (item.state === 'denied' || item.state === 'disabled') { color = 'danger'; state = '未开启'; }
    else if (item.state === 'not_applicable') state = '不适用';
    else if (item.state === 'error') state = '读取失败';
    var label = names[item.key] || item.key || '未知权限';
    var node = el('span', label + ' · ' + state, 'badge bg-' + color + '-lt text-' + color);
    node.style.whiteSpace = 'normal'; node.style.textAlign = 'left'; node.style.overflowWrap = 'anywhere';
    node.title = String(item.key || '') + ' [' + String(item.state || 'unknown') + ']';
    return node;
  }
  function device(snapshot) {
    var card = el('article', null, 'border rounded p-3 mb-2');
    card.appendChild(el('h4', snapshot.device_model || '未知机型'));
    var id = String(snapshot.installation_id || '');
    var meta = el('div', (snapshot.os_version || '未知系统') + ' · App ' + (snapshot.app_version || '-') + ' (' + (snapshot.version_code || '-') + ') · 实例 ' + id.slice(0, 8), 'hint');
    meta.title = id; card.appendChild(meta);
    card.appendChild(el('div', '最近收到：' + time(snapshot.received_at) + '；采集时间（设备）：' + time(snapshot.collected_at), 'hint mb-2'));
    var grouped = new Map();
    (snapshot.items || []).forEach(function (item) {
      var category = item.category || 'other_permission';
      if (!grouped.has(category)) grouped.set(category, []);
      grouped.get(category).push(item);
    });
    if (!grouped.size) card.appendChild(el('div', '此次上报没有权限项', 'hint'));
    grouped.forEach(function (items, category) {
      var section = el(category === 'normal_permission' ? 'details' : 'div', null, 'mt-2');
      section.appendChild(el(category === 'normal_permission' ? 'summary' : 'div', (groups[category] || category) + '（' + items.length + '）', 'mb-1'));
      var tags = el('div', null, 'd-flex flex-wrap gap-2');
      items.forEach(function (item) { tags.appendChild(badge(item)); });
      section.appendChild(tags); card.appendChild(section);
    });
    return card;
  }
  function cancel() { generation++; if (controller) controller.abort(); }
  async function load(userID) {
    cancel();
    var request = generation;
    controller = new AbortController();
    var host = document.getElementById('userPermissionSnapshots');
    host.replaceChildren(el('div', '正在读取设备权限…', 'hint'));
    try {
      var response = await fetch('/admin/api/users/' + encodeURIComponent(userID) + '/permission-snapshots', { signal: controller.signal });
      if (!response.ok) throw new Error('HTTP ' + response.status);
      var body = await response.json();
      if (request !== generation) return;
      var data = body.data || {}, items = data.items || [];
      host.replaceChildren();
      if (!items.length) host.appendChild(el('div', '暂无权限上报；旧版客户端或尚未登录上报的设备不会显示。', 'hint'));
      items.forEach(function (snapshot) { host.appendChild(device(snapshot)); });
      if (items.length >= (data.limit || 100)) host.appendChild(el('div', '仅展示最近上报的 ' + items.length + ' 个安装实例。', 'hint'));
    } catch (error) {
      if (request !== generation || error.name === 'AbortError') return;
      host.replaceChildren(el('div', '权限信息读取失败（' + error.message + '）', 'text-danger mb-2'));
      var retry = el('button', '重新读取', 'btn btn-sm btn-outline-secondary');
      retry.type = 'button'; retry.addEventListener('click', function () { load(userID); }); host.appendChild(retry);
    }
  }
  window.AdminUserPermissions = { load: load, cancel: cancel };
})();
