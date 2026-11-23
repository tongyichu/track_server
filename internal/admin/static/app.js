(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const statusLabels = { published: '已发布', draft: '草稿', archived: '已归档' };
  const canWrite = () => AdminAccess.can('releases', true);
  const publishForm = $('publishForm'), apkFile = $('apkFile');
  const uploadBtn = $('uploadBtn'), publishBtn = $('publishBtn'), togglePublish = $('togglePublish');
  const notesList = $('releaseNotesList'), uploadHint = $('uploadHint'), packageMetadata = $('packageMetadata');
  const statusInput = publishForm.elements.status, minVersion = publishForm.elements.min_supported_version_code;
  let packageInfo = null, upload = null, publishing = false;
  let releases = null, listRequest = null;
  let releaseDetailRoot = null, changelogRoot = null;

  async function request(path, options) {
    const response = await fetch('/admin/api/releases' + path, options);
    const result = await response.json().catch(() => ({}));
    if (!response.ok || result.code !== 0) throw new Error(result.error || '请求失败（' + response.status + '）');
    return result.data;
  }

  function formatSize(bytes) {
    if (!(bytes > 0)) return '—';
    if (bytes >= 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB';
    return (bytes / 1024).toFixed(1) + ' KB';
  }

  function addField(target, label, value, wide) {
    const field = document.createElement('div');
    if (wide) field.className = 'release-detail-wide';
    const term = document.createElement('dt'), detail = document.createElement('dd');
    term.textContent = label; detail.textContent = String(value ?? '—');
    field.append(term, detail); target.appendChild(field);
  }

  function setPublishOpen(open) {
    $('publishPanel').hidden = !open;
    togglePublish.setAttribute('aria-expanded', String(open));
    togglePublish.textContent = open ? '收起发布表单' : '发布新版本';
    if (open) apkFile.focus();
  }

  function setPublishError(message = '') {
    $('publishErr').textContent = message;
    $('publishErr').hidden = !message;
  }

  function updateActions() {
    publishBtn.disabled = publishing || Boolean(upload) || !packageInfo;
    uploadBtn.disabled = publishing || Boolean(upload) || !apkFile.files.length;
    apkFile.disabled = publishing;
    publishForm.inert = publishing;
    togglePublish.disabled = publishing;
    const next = { published: '核对并发布', draft: '保存草稿', archived: '保存为归档' };
    publishBtn.textContent = publishing ? '处理中…' : next[statusInput.value];
  }

  togglePublish.addEventListener('click', () => {
    if (canWrite() && !publishing) setPublishOpen($('publishPanel').hidden);
  });
  statusInput.addEventListener('change', updateActions);

  function clearPackageMetadata() {
    packageInfo = null;
    packageMetadata.replaceChildren();
    packageMetadata.hidden = true;
    minVersion.removeAttribute('max');
    uploadHint.textContent = '';
    setPublishError();
  }

  apkFile.addEventListener('change', () => {
    upload?.abort();
    upload = null;
    clearPackageMetadata();
    updateActions();
  });
  uploadBtn.addEventListener('click', async () => {
    const file = apkFile.files[0];
    if (!canWrite() || !file || upload || publishing) return;
    clearPackageMetadata();
    const controller = upload = new AbortController();
    const body = new FormData();
    body.append('file', file);
    body.append('platform', 'android');
    uploadHint.textContent = '正在上传并解析 · ' + formatSize(file.size);
    updateActions();
    try {
      const data = await request('/upload-package', { method: 'POST', body, signal: controller.signal });
      if (upload !== controller || file !== apkFile.files[0]) return;
      if (!data?.url || !data.version_name || !(data.version_code > 0)) throw new Error('安装包版本信息识别失败，请重新上传。');
      packageInfo = {
        version_name: data.version_name, version_code: data.version_code,
        package_url: data.url, package_size: data.size || file.size, package_md5: data.md5 || ''
      };
      minVersion.max = String(data.version_code);
      addField(packageMetadata, '版本名', data.version_name);
      addField(packageMetadata, '构建号', data.version_code);
      addField(packageMetadata, '包大小', formatSize(packageInfo.package_size));
      if (data.package_name) addField(packageMetadata, '包名', data.package_name);
      packageMetadata.hidden = false;
      uploadHint.textContent = '安装包已就绪';
    } catch (error) {
      if (upload === controller && !controller.signal.aborted) uploadHint.textContent = error.message;
    } finally {
      if (upload === controller) { upload = null; updateActions(); }
    }
  });

  function refreshNoteRowButtons() {
    const rows = Array.from(notesList.querySelectorAll('.release-note-item'));
    rows.forEach((row, index) => {
      const button = row.querySelector('button');
      const add = index === rows.length - 1;
      button.textContent = add ? '+' : '×';
      button.title = add ? '添加一条' : '删除该条';
      button.setAttribute('aria-label', button.title);
      button.classList.toggle('release-note-add', add);
    });
  }

  function addNoteRow() {
    const row = document.createElement('div');
    row.className = 'release-note-item';
    const input = document.createElement('input');
    input.type = 'text';
    input.maxLength = 200;
    input.placeholder = '一条更新内容，如新增轨迹分享';
    input.setAttribute('aria-label', '更新内容');
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'release-note-remove';
    button.addEventListener('click', () => {
      if (button.classList.contains('release-note-add')) addNoteRow().focus();
      else { row.remove(); refreshNoteRowButtons(); }
    });
    row.append(input, button);
    notesList.appendChild(row);
    refreshNoteRowButtons();
    return input;
  }
  addNoteRow();

  function currentRelease(items) { return items.find(item => item.status === 'published'); }
  function releaseName(item) { return item.version_name + '（构建 ' + item.version_code + '）'; }
  function upgradePolicy(item) {
    if (item.force_update) return '低于构建 ' + item.version_code + ' 的客户端须升级。';
    if (item.min_supported_version_code > 0) return '低于构建 ' + item.min_supported_version_code + ' 的客户端须升级，其余旧版可选更新。';
    return '未设最低支持构建号，旧版本可选更新。';
  }
  async function fetchReleases(signal) {
    const data = await request('?platform=android', { signal });
    return (data?.items || []).slice().sort((a, b) => b.version_code - a.version_code);
  }

  publishForm.addEventListener('submit', async event => {
    event.preventDefault();
    if (!canWrite() || !packageInfo || upload || publishing || !publishForm.reportValidity()) return;
    setPublishError();
    const formData = new FormData(publishForm);
    const candidate = {
      ...packageInfo, platform: 'android',
      min_supported_version_code: Number(formData.get('min_supported_version_code')),
      release_notes: Array.from(notesList.querySelectorAll('input'), input => input.value.trim()).filter(Boolean).join('\n'),
      force_update: formData.get('force_update') === 'on', status: statusInput.value
    };
    publishing = true;
    updateActions();
    let submitted = false;
    try {
      const items = await fetchReleases();
      const current = currentRelease(items);
      const existing = items.find(item => item.version_code === candidate.version_code);
      const next = candidate.status === 'published' && (!current || candidate.version_code >= current.version_code);
      const details = [
        releaseName(candidate) + ' · ' + formatSize(candidate.package_size) + '。',
        '最低支持构建号 ' + (candidate.min_supported_version_code || '未设置') + ' · 强制升级' + (candidate.force_update ? '开启' : '关闭') + '。'
      ];
      if (existing) details.push('将覆盖现有的同构建号记录。');
      if (next) details.push('确认后，客户端将收到此版本。' + upgradePolicy(candidate));
      else if (existing === current && existing) {
        const previous = items.find(item => item.status === 'published' && item.id !== existing.id);
        details.push(previous ? '保存后，后续升级将以 ' + releaseName(previous) + ' 为目标。' : '保存后，客户端将没有可下发的版本。');
      } else details.push(current ? '客户端继续下发 ' + releaseName(current) + '。' : '此版本保存后供后台查看。');
      const action = { published: '发布版本', draft: '保存草稿', archived: '保存为归档' }[candidate.status];
      if (!await AdminUI.confirm({
        title: action, message: details.join('\n'), confirmText: action,
        danger: (next && (candidate.force_update || candidate.min_supported_version_code > 0)) || Boolean(existing && existing === current && !next),
        preserveLines: true, waitForHidden: true
      })) return;
      submitted = true;
      await request('', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(candidate) });
      AdminUI.alert(candidate.status === 'published' ? '版本已发布' : candidate.status === 'draft' ? '草稿已保存' : '版本已归档', 'success');
      publishForm.reset();
      clearPackageMetadata();
      notesList.replaceChildren();
      addNoteRow();
      setPublishOpen(false);
      refreshReleaseData();
    } catch (error) {
      setPublishError(submitted && error instanceof TypeError ? '发布结果未确认，请先刷新版本记录核对。' : error.message);
    } finally {
      publishing = false;
      updateActions();
      if ($('publishPanel').hidden) togglePublish.focus();
      else if (document.activeElement === document.body) publishBtn.focus();
    }
  });

  const filterStatus = $('filterStatus'), releaseList = $('releaseList');
  const releaseTable = releaseList.closest('table'), listMessage = $('releaseListMessage');
  function renderReleaseList() {
    if (!releases) return;
    const current = currentRelease(releases);
    $('currentRelease').textContent = current ? '· 当前下发 ' + current.version_name : '· 暂无下发版本';
    const items = releases.filter(item => !filterStatus.value || item.status === filterStatus.value);
    releaseTable.hidden = !items.length;
    listMessage.hidden = Boolean(items.length);
    if (!items.length) listMessage.textContent = releases.length ? '此状态暂无版本' : '暂无版本记录';
    releaseList.replaceChildren();
    items.forEach(item => {
      const row = releaseList.insertRow();
      const version = row.insertCell();
      const detail = document.createElement('button');
      detail.type = 'button';
      detail.className = 'btn btn-link release-version-link';
      detail.textContent = item.version_name || '未命名版本';
      detail.setAttribute('aria-label', '查看 ' + releaseName(item) + ' 的详情');
      detail.addEventListener('click', () => showReleaseDetail(item, detail));
      const meta = document.createElement('span');
      meta.className = 'release-version-meta';
      meta.textContent = '构建 ' + item.version_code + (item.package_size > 0 ? ' · ' + formatSize(item.package_size) : '');
      version.append(detail, meta);
      const state = row.insertCell();
      state.textContent = item === current ? '当前下发' : statusLabels[item.status] || item.status || '—';
      if (item === current) state.className = 'release-current-label';
      row.insertCell().textContent = AdminTime.dateTime(item.updated_at);
      const notes = document.createElement('span');
      notes.className = 'release-notes-preview';
      notes.textContent = String(item.release_notes || '').split('\n').map(line => line.trim()).filter(Boolean).join(' · ') || '—';
      row.insertCell().appendChild(notes);
    });
  }

  async function loadList() {
    listRequest?.abort();
    const controller = listRequest = new AbortController();
    $('refreshBtn').disabled = true;
    listMessage.hidden = false;
    listMessage.textContent = '正在读取版本记录…';
    try {
      const items = await fetchReleases(controller.signal);
      if (listRequest !== controller) return;
      releases = items;
      renderReleaseList();
    } catch (error) {
      if (listRequest !== controller || controller.signal.aborted) return;
      if (!releases) $('currentRelease').textContent = '· 当前版本读取失败';
      listMessage.hidden = false;
      listMessage.textContent = '版本记录读取失败。' + (releases ? '当前显示上次数据。' : '') + '可点击刷新重试。';
    } finally {
      if (listRequest === controller) { listRequest = null; $('refreshBtn').disabled = false; }
    }
  }

  $('refreshBtn').addEventListener('click', loadList);
  filterStatus.addEventListener('change', renderReleaseList);

  function showReleaseDetail(item, trigger) {
    if (releaseDetailRoot) return;
    const root = releaseDetailRoot = document.createElement('dialog');
    root.className = 'modal release-detail';
    root.setAttribute('aria-labelledby', 'releaseDetailTitle');
    root.innerHTML = '<div class="modal-dialog modal-dialog-centered modal-dialog-scrollable modal-lg"><div class="modal-content">' +
      '<div class="modal-header"><div><h2 id="releaseDetailTitle" class="modal-title"></h2><p class="hint mb-0"></p></div>' +
      '<button type="button" class="btn-close" aria-label="关闭"></button></div><div class="modal-body"></div>' +
      '<div class="modal-footer"></div></div></div>';
    root.querySelector('.modal-title').textContent = releaseName(item);
    root.querySelector('.hint').textContent = [
      item === currentRelease(releases || []) ? '当前下发' : statusLabels[item.status] || item.status,
      item.operator_name, AdminTime.dateTime(item.updated_at)
    ].filter(Boolean).join(' · ');

    const fields = document.createElement('dl');
    fields.className = 'release-detail-fields';
    addField(fields, '包大小', formatSize(item.package_size));
    addField(fields, '最低支持构建号', item.min_supported_version_code || '未设门槛');
    addField(fields, '强制升级', item.force_update ? '开启' : '关闭');
    addField(fields, '记录 ID', item.id);
    addField(fields, '平台', item.platform || '—');
    addField(fields, '创建时间', AdminTime.dateTime(item.created_at));
    addField(fields, '包 MD5', item.package_md5 || '—', true);

    const notes = document.createElement('section');
    notes.className = 'release-detail-notes';
    const title = document.createElement('h3');
    title.textContent = '更新说明';
    const lines = document.createElement('div');
    renderNoteLines(lines, item.release_notes);
    notes.append(title, lines);
    root.querySelector('.modal-body').append(fields, notes);
    const footer = root.querySelector('.modal-footer');
    if (item.package_url) {
      let url;
      try { url = new URL(item.package_url, location.origin); }
      catch (_) { url = null; }
      if (url && ['http:', 'https:'].includes(url.protocol)) {
        const download = document.createElement('a');
        download.className = 'btn btn-outline-primary';
        download.href = url.href;
        download.rel = 'noopener';
        download.target = '_blank';
        download.textContent = '下载安装包';
        footer.prepend(download);
      }
    }
    document.body.appendChild(root);
    const modal = new AdminComponents.Dialog(root);
    let removing = false;
    root.querySelector('.btn-close').addEventListener('click', () => modal.hide());
    if (canWrite()) {
      const remove = document.createElement('button');
      remove.type = 'button'; remove.className = 'btn btn-outline-danger';
      remove.dataset.adminWrite = 'releases'; remove.textContent = '删除版本';
      remove.addEventListener('click', () => { removing = true; modal.hide(); });
      footer.prepend(remove);
    }
    footer.hidden = !footer.childElementCount;
    root.addEventListener('admin:dialog-closed', () => {
      releaseDetailRoot = null;
      modal.dispose(); root.remove();
      if (removing) AdminUI.runOnce('remove-release', () => deleteRelease(item, trigger));
      else trigger?.focus();
    });
    modal.show();
  }

  async function deleteRelease(item, trigger) {
    if (!canWrite()) return;
    try {
      const items = await fetchReleases();
      const selected = items.find(record => record.id === item.id);
      if (!selected) { refreshReleaseData(); AdminUI.alert('该版本已不存在', 'warning'); return; }
      const published = items.filter(record => record.status === 'published');
      const details = ['删除 ' + releaseName(selected) + ' 后，将清理服务端本地安装包。更新日志会保留。'];
      if (published[0] === selected) {
        details.push(published[1] ? '后续升级将以 ' + releaseName(published[1]) + ' 为目标。' : '客户端将没有可下发的版本。');
      }
      if (!await AdminUI.confirm({ title: '删除版本', message: details.join('\n'), confirmText: '删除版本', danger: true, preserveLines: true, waitForHidden: true })) {
        showReleaseDetail(selected, trigger);
        return;
      }
      await request('/' + encodeURIComponent(selected.id), { method: 'DELETE' });
      AdminUI.alert('版本已删除', 'success');
      refreshReleaseData();
    } catch (error) {
      AdminUI.alert(error instanceof TypeError ? '无法确认删除结果，请刷新版本记录核对。' : error.message, 'danger');
    }
  }

  // ---------------- 更新日志弹窗（独立留存） ----------------
  async function showChangelogModal() {
    if (changelogRoot) return;
    const root = changelogRoot = document.createElement('dialog');
    const controller = new AbortController();
    root.className = 'modal';
    root.setAttribute('aria-label', '更新日志');
    root.innerHTML = '<div class="modal-dialog modal-dialog-centered modal-dialog-scrollable modal-lg"><div class="modal-content">' +
      '<div class="modal-header"><h5 class="modal-title">更新日志</h5><button type="button" class="btn-close" aria-label="关闭"></button></div>' +
      '<div class="modal-body"><div class="changelog-loading">加载中…</div></div>' +
      '</div></div>';
    document.body.appendChild(root);
    const modal = new AdminComponents.Dialog(root);
    root.querySelector('.btn-close').addEventListener('click', function () { modal.hide(); });
    root.addEventListener('admin:dialog-closing', function () { controller.abort(); });
    root.addEventListener('admin:dialog-closed', function () { changelogRoot = null; modal.dispose(); root.remove(); });
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

  const csSection = $('clientStats'), csDays = $('clientStatsDays');
  const csRefresh = $('refreshClientStats'), csError = $('clientStatsError');
  const charts = new Map();
  let statsRequest = null, statsStale = true;

  function setChart(id, options) {
    if (!charts.has(id)) charts.set(id, echarts.init($(id)));
    charts.get(id).setOption(options, true);
  }

  function versionLabel(item) { return String(item.version_name || item.label || '').trim() || '未知'; }
  function share(count, total) { return (total > 0 ? count * 100 / total : 0).toFixed(1) + '%'; }

  function renderDimensions(id, distribution) {
    const rows = distribution.slice(0, 10).map(item => ({ name: item.label, value: item.devices }));
    if (distribution.length > 10) rows.push({
      name: '其他', value: distribution.slice(10).reduce((sum, item) => sum + item.devices, 0)
    });
    setChart(id, AdminChartStyle.bars(rows, 0, { unit: '台', labelWidth: 115 }));
  }

  function renderClientCharts(data) {
    const total = data.active_devices || 0, latest = data.latest;
    const totals = new Map();
    for (const item of data.version_distribution || []) {
      const name = versionLabel(item);
      totals.set(name, (totals.get(name) || 0) + item.devices);
    }
    const currentName = latest ? versionLabel(latest) : '';
    const versions = Array.from(totals, ([name, value]) => ({ name, value })).sort((a, b) =>
      Number(b.name === currentName) - Number(a.name === currentName) || b.value - a.value
    );
    const ring = AdminChartStyle.ring(versions, total, '活跃设备', '台');
    ring.tooltip.formatter = item => item.name + '\n' + item.value + ' 台 · ' + share(item.value, total);
    setChart('csVersionChart', ring);
    AdminChartStyle.legend($('csVersionLegend'), versions);
    renderDimensions('csModelChart', data.device_model_distribution || []);
    renderDimensions('csOsChart', data.os_version_distribution || []);

    const days = data.daily_trend || [];
    const current = [], earlier = [];
    let inconsistent = false;
    for (const day of days) {
      let currentCount = 0, otherCount = 0;
      for (const item of day.versions || []) {
        if (latest && item.version_code === latest.version_code) currentCount += item.devices;
        else otherCount += item.devices;
      }
      current.push(currentCount);
      earlier.push(otherCount);
      if (currentCount + otherCount > total) inconsistent = true;
    }
    const note = $('csTrendNote');
    note.hidden = Boolean(days.length) && !inconsistent;
    note.textContent = !days.length ? '当前周期暂无趋势数据'
      : '部分历史日期的设备数超过周期去重总量，趋势数据仅供参考。';
    const series = [{
      name: latest ? '其他版本' : '全部版本',
      type: 'line', stack: 'devices', showSymbol: false, data: earlier, areaStyle: { opacity: 0.16 }
    }];
    if (latest) series.push({
      name: currentName + '（当前）',
      type: 'line', stack: 'devices', showSymbol: false, data: current, areaStyle: { opacity: 0.2 }
    });
    setChart('csTrendChart', {
      color: ['#868e96', AdminChartStyle.colors[0]],
      aria: { enabled: true },
      tooltip: { trigger: 'axis', confine: true, renderMode: 'richText' },
      legend: { bottom: 0 },
      grid: { left: 8, right: 14, top: 14, bottom: 28, containLabel: true },
      xAxis: { type: 'category', boundaryGap: false, data: days.map(day => day.date), axisLabel: { formatter: date => date.slice(5) } },
      yAxis: { type: 'value', minInterval: 1 },
      series
    });
  }

  function renderClientMetrics(data) {
    const total = data.active_devices || 0, latest = data.latest;
    $('csActiveDevices').textContent = total + ' 台';
    $('csActiveNote').textContent = '近 ' + (data.days || csDays.value) + ' 天去重设备';
    $('csLatestDevices').textContent = latest ? latest.active_devices + ' 台' : '—';
    $('csLatestNote').textContent = latest ? versionLabel(latest) + ' · ' + share(latest.active_devices, total) : '暂无已发布版本';
    $('csBelowMin').textContent = latest ? latest.below_min_supported + ' 台' : '—';
    $('csBelowMinNote').textContent = latest?.min_supported_version_code > 0
      ? '低于构建 ' + latest.min_supported_version_code : '未设最低版本门槛';
  }

  async function loadClientStats() {
    if (!csSection.open) return;
    statsStale = true;
    statsRequest?.abort();
    const controller = statsRequest = new AbortController();
    csError.hidden = true;
    csRefresh.disabled = csDays.disabled = true;
    csSection.setAttribute('aria-busy', 'true');
    try {
      const data = await request('/client-statistics?days=' + csDays.value, { signal: controller.signal });
      if (statsRequest !== controller || !csSection.open) return;
      renderClientMetrics(data);
      await AdminAssets.loadCharts();
      if (statsRequest !== controller || !csSection.open) return;
      renderClientCharts(data);
      statsStale = false;
    } catch (error) {
      if (controller.signal.aborted || statsRequest !== controller) return;
      csError.textContent = '统计加载失败。' + error.message;
      csError.hidden = false;
    } finally {
      if (statsRequest === controller) {
        statsRequest = null;
        csRefresh.disabled = csDays.disabled = false;
        csSection.removeAttribute('aria-busy');
      }
    }
  }

  function resizeCharts() {
    if (csSection.open) charts.forEach(chart => chart.resize());
  }
  csSection.addEventListener('toggle', () => {
    if (!csSection.open) {
      if (statsRequest) statsStale = true;
      statsRequest?.abort();
      return;
    }
    if (statsStale) loadClientStats();
    requestAnimationFrame(resizeCharts);
  });
  csDays.addEventListener('change', loadClientStats);
  csRefresh.addEventListener('click', loadClientStats);
  window.addEventListener('resize', resizeCharts);
  function refreshReleaseData() {
    statsStale = true;
    loadList();
    if (csSection.open) loadClientStats();
  }
  loadList();
})();
