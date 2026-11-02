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
    metadataHint.textContent = '上传后自动识别版本信息，并生成下载地址、文件大小和 MD5。';
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
      release_notes: fd.get('release_notes') || '',
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
      const notes = (it.release_notes || '').trim();
      const firstLine = notes.split('\n').map(function (s) { return s.trim(); }).filter(Boolean)[0] || '';
      const tr = document.createElement('tr');
      tr.innerHTML = '<td>' + it.id + '</td>' +
        '<td>' + esc(it.version_name) + '</td>' +
        '<td>' + it.version_code + '</td>' +
        '<td>' + it.min_supported_version_code + '</td>' +
        '<td>' + (it.force_update ? '是' : '否') + '</td>' +
        '<td>' + esc(it.status) + '</td>' +
        '<td>' + esc(it.operator_name || '') + '</td>' +
        '<td>' + esc((it.updated_at || '').replace('T', ' ').slice(0, 19)) + '</td>' +
        '<td>' + formatSize(it.package_size) + '</td>' +
        '<td class="release-notes-cell"></td>' +
        '<td class="url" title="' + esc(it.package_url) + '"><a href="' + esc(it.package_url) + '" target="_blank">' + esc(it.package_url) + '</a></td>' +
        '<td><button data-admin-write="releases" class="danger" data-id="' + it.id + '">删除</button></td>';
      const notesCell = tr.querySelector('.release-notes-cell');
      if (notes) {
        const summary = document.createElement('span');
        summary.textContent = firstLine.length > 30 ? firstLine.slice(0, 30) + '…' : firstLine;
        notesCell.appendChild(summary);
        notesCell.appendChild(document.createTextNode(' '));
        const toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.className = 'link';
        toggle.textContent = '展开';
        let expandedRow = null;
        toggle.addEventListener('click', function () {
          if (expandedRow) {
            expandedRow.remove();
            expandedRow = null;
            toggle.textContent = '展开';
            return;
          }
          expandedRow = document.createElement('tr');
          expandedRow.className = 'release-notes-expanded';
          const td = document.createElement('td');
          td.colSpan = 12;
          const pre = document.createElement('pre');
          pre.className = 'release-notes-full';
          pre.textContent = notes;
          td.appendChild(pre);
          expandedRow.appendChild(td);
          tr.after(expandedRow);
          toggle.textContent = '收起';
        });
        notesCell.appendChild(toggle);
      } else {
        notesCell.textContent = '—';
      }
      tbody.appendChild(tr);
    });
    tbody.querySelectorAll('button.danger').forEach(function (b) {
      b.addEventListener('click', async function () {
        if (!await AdminUI.confirm({ title: '删除发布版本', message: '确认删除版本 #' + b.dataset.id + '？', confirmText: '删除版本', danger: true })) return;
        const r = await fetch('/admin/api/releases/' + b.dataset.id, { method: 'DELETE' });
        if (!r.ok) { AdminUI.alert('删除失败', 'danger'); return; }
        AdminUI.alert('发布版本已删除', 'success');
        loadList();
      });
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
    if (item.label && item.label !== '未知' && item.version_code > 0) {
      return item.label + ' (' + item.version_code + ')';
    }
    if (item.label && item.label !== '未知') return item.label;
    if (item.version_code > 0) return 'Build ' + item.version_code;
    return '未知';
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
    const versionItems = (data.version_distribution || []).map(function (it) {
      return { name: csVersionLabel(it), value: it.devices };
    });
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
      const seriesMap = {};
      (data.daily_trend || []).forEach(function (day) {
        dates.push(day.date);
        (day.versions || []).forEach(function (v) {
          const name = csVersionLabel(v);
          if (!seriesMap[name]) seriesMap[name] = {};
          seriesMap[name][day.date] = v.devices;
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
          latest.version_name + ' (' + latest.version_code + ') · 占比 ' + latest.share_percent + '%';
        document.getElementById('csBelowMin').textContent = latest.below_min_supported + ' 台';
        document.getElementById('csBelowMinNote').textContent = '版本号低于 ' + latest.min_supported_version_code;
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
