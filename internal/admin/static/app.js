(function () {
  // 当前会话信息

  // -- 登录态校验 --

  // -- 退出 --

  // ---------------- 本机上传 ----------------
  // 安装包直接 multipart 上传到服务端 <staticRoot>/release/<platform>/，
  // 服务端返回的 url 形如 "/api/v1/static/release/android/<filename>"，
  // 直接填入 package_url 即可。
  const apkFile = document.getElementById('apkFile');
  const uploadBtn = document.getElementById('uploadBtn');
  const uploadHint = document.getElementById('uploadHint');
  const pkgURLInput = document.querySelector('input[name=package_url]');
  const pkgSizeInput = document.querySelector('input[name=package_size]');
  const pkgMD5Input = document.querySelector('input[name=package_md5]');
  const versionNameInput = document.querySelector('input[name=version_name]');
  const versionCodeInput = document.querySelector('input[name=version_code]');
  const metadataHint = document.getElementById('packageMetadataHint');

  function clearPackageMetadata() {
    pkgURLInput.value = '';
    pkgSizeInput.value = '0';
    pkgMD5Input.value = '';
    versionNameInput.value = '';
    versionCodeInput.value = '';
    uploadHint.textContent = '';
    metadataHint.textContent = '';
  }
  apkFile.addEventListener('change', clearPackageMetadata);

  uploadBtn.addEventListener('click', async function () {
    const file = apkFile.files[0];
    if (!file) {
      uploadHint.textContent = '请先选择安装包文件';
      return;
    }
    uploadHint.textContent = '正在上传 (' + (file.size / 1024 / 1024).toFixed(2) + ' MB) ...';
    try {
      const fd = new FormData();
      fd.append('platform', 'android');
      fd.append('file', file);
      const resp = await fetch('/admin/api/releases/upload-package', {
        method: 'POST',
        body: fd,
      });
      if (!resp.ok) {
        const j = await resp.json().catch(function () { return {}; });
        throw new Error('上传失败: ' + (j.error || resp.status));
      }
      const data = ((await resp.json()).data) || {};
      if (!data.url) {
        throw new Error('服务端未返回 url 字段');
      }
      pkgURLInput.value = data.url;
      pkgSizeInput.value = String(data.size || file.size);
      pkgMD5Input.value = data.md5 || '';
      if (!data.version_name || !(data.version_code > 0)) {
        clearPackageMetadata();
        throw new Error('安装包版本信息识别失败，无法发布');
      }
      versionNameInput.value = data.version_name;
      versionCodeInput.value = String(data.version_code);
      metadataHint.textContent = [data.package_name, '版本 ' + data.version_name, 'Build ' + data.version_code].filter(Boolean).join(' · ');
      uploadHint.textContent = '安装包上传并解析成功';
    } catch (e) {
      uploadHint.textContent = e.message || String(e);
    }
  });

  // ---------------- 发布 ----------------
  const publishForm = document.getElementById('publishForm');
  const publishErr = document.getElementById('publishErr');
  const publishOk = document.getElementById('publishOk');
  const notesList = document.getElementById('releaseNotesList');

  function refreshNoteRowButtons() {
    const rows = Array.from(notesList.querySelectorAll('.release-note-item'));
    rows.forEach(function (row, index) {
      const btn = row.querySelector('button');
      const isLast = index === rows.length - 1;
      btn.textContent = isLast ? '+' : '×';
      btn.title = isLast ? '添加一条' : '删除该条';
      btn.classList.toggle('release-note-add', isLast);
    });
  }

  function addNoteRow(value) {
    const row = document.createElement('div');
    row.className = 'release-note-item';
    const input = document.createElement('input');
    input.type = 'text';
    input.maxLength = 200;
    input.placeholder = '一条更新内容，如：新增轨迹分享';
    input.value = value || '';
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'release-note-remove';
    btn.addEventListener('click', function () {
      if (btn.classList.contains('release-note-add')) {
        addNoteRow().focus();
        return;
      }
      row.remove();
      if (!notesList.querySelector('.release-note-item')) addNoteRow();
      refreshNoteRowButtons();
    });
    row.append(input, btn);
    notesList.appendChild(row);
    refreshNoteRowButtons();
    return input;
  }

  function collectReleaseNotes() {
    return Array.from(notesList.querySelectorAll('.release-note-item input'))
      .map(function (input) { return input.value.trim(); })
      .filter(Boolean)
      .join('\n');
  }

  function resetReleaseNotes() {
    notesList.innerHTML = '';
    addNoteRow();
  }

  resetReleaseNotes();

  publishForm.addEventListener('submit', async function (e) {
    e.preventDefault();
    publishErr.textContent = '';
    publishOk.textContent = '';
    const fd = new FormData(publishForm);
    const payload = {
      platform: fd.get('platform'),
      version_name: (fd.get('version_name') || '').trim(),
      version_code: parseInt(fd.get('version_code') || '0', 10),
      min_supported_version_code: parseInt(fd.get('min_supported_version_code') || '0', 10),
      package_url: (fd.get('package_url') || '').trim(),
      package_size: parseInt(fd.get('package_size') || '0', 10),
      package_md5: (fd.get('package_md5') || '').trim(),
      release_notes: collectReleaseNotes(),
      force_update: fd.get('force_update') === 'on',
      status: fd.get('status') || 'published',
    };
    if (!pkgURLInput.value) {
      publishErr.textContent = '请先选择并上传 APK';
      return;
    }
    if (!payload.version_name || !(payload.version_code > 0)) {
      publishErr.textContent = '安装包版本信息识别失败，请重新上传';
      return;
    }
    try {
      const resp = await fetch('/admin/api/releases', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!resp.ok) {
        const j = await resp.json().catch(function () { return {}; });
        publishErr.textContent = j.error || ('发布失败: ' + resp.status);
        return;
      }
      publishOk.textContent = '发布成功';
      resetReleaseNotes();
      loadList();
    } catch (e) {
      publishErr.textContent = e.message || String(e);
    }
  });

  // ---------------- 列表 ----------------
  const filterStatus = document.getElementById('filterStatus');
  document.getElementById('refreshBtn').addEventListener('click', loadList);
  filterStatus.addEventListener('change', loadList);

  function formatSize(bytes) {
    if (!(bytes > 0)) return '—';
    if (bytes >= 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB';
    return (bytes / 1024).toFixed(1) + ' KB';
  }

  let releaseDetailRoot = null, changelogRoot = null;
  function showReleaseDetail(it) {
    if (releaseDetailRoot) return;
    const root = releaseDetailRoot = document.createElement('div');
    root.className = 'modal modal-blur fade';
    root.tabIndex = -1;
    root.innerHTML = '<div class="modal-dialog modal-dialog-centered modal-dialog-scrollable modal-lg"><div class="modal-content">' +
      '<div class="modal-header"><h5 class="modal-title">版本详情</h5><button type="button" class="btn-close" aria-label="关闭"></button></div>' +
      '<div class="modal-body"></div>' +
      '<div class="modal-footer"><button type="button" class="btn btn-outline-secondary">关闭</button></div>' +
      '</div></div>';
    const rows = [
      ['ID', String(it.id)],
      ['平台', it.platform || ''],
      ['版本名', it.version_name || ''],
      ['版本号', String(it.version_code)],
      ['最低支持版本号', String(it.min_supported_version_code)],
      ['强制升级', it.force_update ? '是' : '否'],
      ['状态', it.status || ''],
      ['发布人', it.operator_name || '—'],
      ['创建时间', AdminTime.dateTime(it.created_at)],
      ['更新时间', AdminTime.dateTime(it.updated_at)],
      ['包大小', formatSize(it.package_size)],
      ['包 MD5', it.package_md5 || '—'],
    ];
    const table = document.createElement('table');
    table.className = 'data release-detail-table';
    rows.forEach(function (pair) {
      const tr = document.createElement('tr');
      const th = document.createElement('th');
      th.textContent = pair[0];
      const td = document.createElement('td');
      td.textContent = pair[1];
      if (pair[0] === '包 MD5') td.className = 'mono';
      tr.append(th, td);
      table.appendChild(tr);
    });
    const urlTr = document.createElement('tr');
    const urlTh = document.createElement('th');
    urlTh.textContent = '包地址';
    const urlTd = document.createElement('td');
    const link = document.createElement('a');
    link.href = it.package_url || '#';
    link.target = '_blank';
    link.textContent = it.package_url || '—';
    urlTd.appendChild(link);
    urlTr.append(urlTh, urlTd);
    table.appendChild(urlTr);
    const notesTr = document.createElement('tr');
    const notesTh = document.createElement('th');
    notesTh.textContent = '更新说明';
    const notesTd = document.createElement('td');
    const notesScroll = document.createElement('div');
    notesScroll.className = 'release-notes-detail';
    renderNoteLines(notesScroll, it.release_notes);
    notesTd.appendChild(notesScroll);
    notesTr.append(notesTh, notesTd);
    table.appendChild(notesTr);
    root.querySelector('.modal-body').appendChild(table);
    document.body.appendChild(root);
    const modal = new tabler.Modal(root);
    root.querySelector('.btn-close').addEventListener('click', function () { modal.hide(); });
    root.querySelector('.modal-footer button').addEventListener('click', function () { modal.hide(); });
    root.addEventListener('hidden.bs.modal', function () { releaseDetailRoot = null; modal.dispose(); root.remove(); });
    modal.show();
  }

  async function loadList() {
    const params = new URLSearchParams();
    params.set('platform', 'android');
    if (filterStatus.value) params.set('status', filterStatus.value);
    const resp = await fetch('/admin/api/releases?' + params.toString());
    if (!resp.ok) return;
    const items = ((await resp.json()).data || {}).items || [];
    const tbody = document.getElementById('releaseList');
    tbody.innerHTML = '';
    items.forEach(function (it) {
      const tr = document.createElement('tr');
      tr.innerHTML = '<td>' + it.id + '</td>' +
        '<td>' + esc(it.version_name) + '</td>' +
        '<td>' + it.min_supported_version_code + '</td>' +
        '<td>' + (it.force_update ? '是' : '否') + '</td>' +
        '<td>' + esc(it.status) + '</td>' +
        '<td>' + esc(it.operator_name || '') + '</td>' +
        '<td>' + esc(AdminTime.dateTime(it.updated_at)) + '</td>' +
        '<td>' + formatSize(it.package_size) + '</td>' +
        '<td class="release-notes-cell"></td>' +
        '<td class="row-actions"></td>';
      const notesCell = tr.querySelector('.release-notes-cell');
      renderNoteLines(notesCell, it.release_notes);
      const actions = tr.querySelector('.row-actions');
      const detailBtn = document.createElement('button');
      detailBtn.type = 'button';
      detailBtn.textContent = '查看详情';
      detailBtn.addEventListener('click', function () { showReleaseDetail(it); });
      const deleteBtn = document.createElement('button');
      deleteBtn.type = 'button';
      deleteBtn.className = 'danger';
      deleteBtn.dataset.id = it.id;
      deleteBtn.setAttribute('data-admin-write', 'releases');
      deleteBtn.textContent = '删除';
      actions.append(detailBtn, deleteBtn);
      tbody.appendChild(tr);
    });
    tbody.querySelectorAll('button.danger').forEach(function (b) {
      b.addEventListener('click', async function () {
        if (!await AdminUI.confirm({ title: '删除发布版本', message: '确认删除版本 #' + b.dataset.id + '？将同时删除安装包文件，更新日志会保留。', confirmText: '删除版本', danger: true })) return;
        const r = await fetch('/admin/api/releases/' + b.dataset.id, { method: 'DELETE' });
        if (!r.ok) { AdminUI.alert('删除失败', 'danger'); return; }
        AdminUI.alert('发布版本已删除', 'success');
        loadList();
      });
    });
  }

  // ---------------- 更新日志弹窗（独立留存） ----------------
  async function showChangelogModal() {
    if (changelogRoot) return;
    const root = changelogRoot = document.createElement('div');
    const controller = new AbortController();
    root.className = 'modal modal-blur fade';
    root.tabIndex = -1;
    root.innerHTML = '<div class="modal-dialog modal-dialog-centered modal-dialog-scrollable modal-lg"><div class="modal-content">' +
      '<div class="modal-header"><h5 class="modal-title">更新日志</h5><button type="button" class="btn-close" aria-label="关闭"></button></div>' +
      '<div class="modal-body"><div class="changelog-loading">加载中…</div></div>' +
      '<div class="modal-footer"><button type="button" class="btn btn-outline-secondary">关闭</button></div>' +
      '</div></div>';
    document.body.appendChild(root);
    const modal = new tabler.Modal(root);
    root.querySelector('.btn-close').addEventListener('click', function () { modal.hide(); });
    root.querySelector('.modal-footer button').addEventListener('click', function () { modal.hide(); });
    root.addEventListener('hide.bs.modal', function () { controller.abort(); });
    root.addEventListener('hidden.bs.modal', function () { changelogRoot = null; modal.dispose(); root.remove(); });
    modal.show();

    const body = root.querySelector('.modal-body');
    try {
      const resp = await fetch('/admin/api/releases/changelogs?platform=android', { signal: controller.signal });
      if (!resp.ok) throw new Error('加载失败: ' + resp.status);
      const items = ((await resp.json()).data || {}).items || [];
      if (controller.signal.aborted) return;
      body.innerHTML = '';
      if (items.length === 0) {
        body.innerHTML = '<div class="changelog-loading">暂无更新日志</div>';
        return;
      }
      items.forEach(function (it) {
        const entry = document.createElement('div');
        entry.className = 'changelog-entry';
        const head = document.createElement('div');
        head.className = 'changelog-entry-head';
        const version = document.createElement('strong');
        version.textContent = it.version_name || '';
        const meta = document.createElement('span');
        meta.className = 'changelog-entry-meta';
        const time = AdminTime.dateTime(it.created_at);
        meta.textContent = [it.operator_name, time].filter(Boolean).join(' · ');
        head.append(version, meta);
        const notes = document.createElement('div');
        notes.className = 'changelog-entry-notes';
        renderNoteLines(notes, it.release_notes);
        entry.append(head, notes);
        body.appendChild(entry);
      });
    } catch (e) {
      if (controller.signal.aborted) return;
      body.innerHTML = '<div class="changelog-loading">' + esc(e.message || String(e)) + '</div>';
    }
  }

  document.getElementById('openChangelogs').addEventListener('click', showChangelogModal);

  function renderNoteLines(cell, notes) {
    const lines = String(notes || '').split('\n').map(function (s) { return s.trim(); }).filter(Boolean);
    if (lines.length === 0) {
      cell.textContent = '—';
      return;
    }
    lines.forEach(function (line) {
      const item = document.createElement('div');
      item.className = 'release-note-line';
      item.textContent = line;
      cell.appendChild(item);
    });
  }

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  // ---------------- 客户端统计 ----------------
  const csSection = document.getElementById('clientStats');
  const csDays = document.getElementById('clientStatsDays');
  const csRefresh = document.getElementById('refreshClientStats');
  const csError = document.getElementById('clientStatsError');
  const csChartIds = ['csTrendChart', 'csVersionChart', 'csModelChart', 'csOsChart'];
  const csCharts = {};

  function csChart(id) {
    if (!window.echarts) return null;
    const el = document.getElementById(id);
    if (!el) return null;
    if (!csCharts[id]) csCharts[id] = echarts.init(el);
    return csCharts[id];
  }

  function csVersionLabel(item) {
    return String(item.version_name || item.label || '').trim() || '未知';
  }

  function csTopWithOther(items, limit) {
    const list = (items || []).slice();
    if (list.length <= limit) return list;
    const top = list.slice(0, limit);
    const rest = list.slice(limit).reduce(function (sum, it) { return sum + (it.devices || 0); }, 0);
    top.push({ label: '其他', devices: rest });
    return top;
  }

  function csRenderBar(id, items) {
    const chart = csChart(id);
    if (!chart) return;
    const labels = items.map(function (it) { return it.label; }).reverse();
    const values = items.map(function (it) { return it.devices; }).reverse();
    chart.setOption({
      grid: { left: 8, right: 30, top: 8, bottom: 8, containLabel: true },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'value', minInterval: 1 },
      yAxis: { type: 'category', data: labels, axisLabel: { width: 110, overflow: 'truncate' } },
      series: [{ type: 'bar', data: values, barMaxWidth: 18, itemStyle: { color: '#206bc4', borderRadius: [0, 4, 4, 0] }, label: { show: true, position: 'right', color: '#667085' } }],
    }, true);
  }

  function csRenderCharts(data) {
    const versionTotals = new Map();
    (data.version_distribution || []).forEach(function (it) {
      const name = csVersionLabel(it);
      versionTotals.set(name, (versionTotals.get(name) || 0) + (it.devices || 0));
    });
    const versionItems = Array.from(versionTotals, function (entry) { return { name: entry[0], value: entry[1] }; });
    const versionChart = csChart('csVersionChart');
    if (versionChart) {
      versionChart.setOption({
        tooltip: { trigger: 'item', formatter: '{b}<br/>{c} 台 ({d}%)' },
        legend: { bottom: 0, type: 'scroll' },
        series: [{
          type: 'pie', radius: ['42%', '68%'], center: ['50%', '44%'],
          itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
          label: { formatter: '{b}\n{d}%' },
          data: versionItems,
        }],
      }, true);
    }

    csRenderBar('csModelChart', csTopWithOther(data.device_model_distribution || [], 10));
    csRenderBar('csOsChart', csTopWithOther(data.os_version_distribution || [], 10));

    const trendChart = csChart('csTrendChart');
    if (trendChart) {
      const dates = [];
      const seriesMap = Object.create(null);
      (data.daily_trend || []).forEach(function (day) {
        dates.push(day.date);
        (day.versions || []).forEach(function (v) {
          const name = csVersionLabel(v);
          if (!seriesMap[name]) seriesMap[name] = Object.create(null);
          seriesMap[name][day.date] = (seriesMap[name][day.date] || 0) + (v.devices || 0);
        });
      });
      const series = Object.keys(seriesMap).map(function (name) {
        return {
          name: name, type: 'line', stack: 'devices', smooth: true, showSymbol: false,
          areaStyle: { opacity: 0.25 }, emphasis: { focus: 'series' },
          data: dates.map(function (d) { return seriesMap[name][d] || 0; }),
        };
      });
      trendChart.setOption({
        tooltip: { trigger: 'axis' },
        legend: { bottom: 0, type: 'scroll' },
        grid: { left: 8, right: 16, top: 16, bottom: 34, containLabel: true },
        xAxis: { type: 'category', boundaryGap: false, data: dates },
        yAxis: { type: 'value', minInterval: 1 },
        series: series,
      }, true);
    }
  }

  async function loadClientStats() {
    if (!csSection) return;
    csError.hidden = true;
    csRefresh.disabled = true;
    csDays.disabled = true;
    csSection.setAttribute('aria-busy', 'true');
    try {
      const resp = await fetch('/admin/api/releases/client-statistics?days=' + csDays.value);
      if (!resp.ok) {
        const j = await resp.json().catch(function () { return {}; });
        throw new Error(j.error || ('统计加载失败: ' + resp.status));
      }
      const data = (await resp.json()).data || {};
      document.getElementById('csActiveDevices').textContent = data.active_devices || 0;
      document.getElementById('csActiveNote').textContent = '近 ' + (data.days || csDays.value) + ' 天去重设备';
      const latest = data.latest || null;
      if (latest) {
        document.getElementById('csLatestDevices').textContent = latest.active_devices + ' 台';
        document.getElementById('csLatestNote').textContent =
          csVersionLabel(latest) + ' · 占比 ' + latest.share_percent + '%';
        document.getElementById('csBelowMin').textContent = latest.below_min_supported + ' 台';
        document.getElementById('csBelowMinNote').textContent = latest.min_supported_version_code > 0 ? '需更新至支持版本' : '暂无强升线';
      } else {
        document.getElementById('csLatestDevices').textContent = '—';
        document.getElementById('csLatestNote').textContent = '暂无已发布版本';
        document.getElementById('csBelowMin').textContent = '—';
        document.getElementById('csBelowMinNote').textContent = '暂无强升线';
      }
      await AdminAssets.loadCharts();
      csRenderCharts(data);
    } catch (e) {
      csError.textContent = e.message || String(e);
      csError.hidden = false;
    } finally {
      csRefresh.disabled = false;
      csDays.disabled = false;
      csSection.setAttribute('aria-busy', 'false');
    }
  }

  if (csSection) {
    csRefresh.addEventListener('click', loadClientStats);
    csDays.addEventListener('change', loadClientStats);
    window.addEventListener('resize', function () {
      csChartIds.forEach(function (id) { if (csCharts[id]) csCharts[id].resize(); });
    });
    loadClientStats();
  }

  loadList();
})();
