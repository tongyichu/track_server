(function () {
  fetch('/admin/api/me').then(function (r) {
    if (!r.ok) { window.location.href = '/admin/login.html'; return null; }
    return r.json();
  }).then(function (j) {
    if (j && j.data) {
      var el = document.getElementById('userName');
      if (el) el.textContent = '👤 ' + j.data.username;
    }
  });

  document.getElementById('logoutBtn').addEventListener('click', async function () {
    await fetch('/admin/api/logout', { method: 'POST' });
    window.location.href = '/admin/login.html';
  });

  var pageSize = 20;
  var offsetStack = [0];
  var pageIndex = 0;
  var lastNextOffset = 0;
  var hasMore = false;
  var currentID = 0;

  var tbody = document.getElementById('summaryList');
  var statusFilter = document.getElementById('statusFilter');
  var refreshBtn = document.getElementById('refreshBtn');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var pageHint = document.getElementById('pageHint');
  var totalHint = document.getElementById('totalHint');
  var emptyDetail = document.getElementById('emptyDetail');
  var detailContent = document.getElementById('detailContent');

  statusFilter.addEventListener('change', resetAndLoad);
  refreshBtn.addEventListener('click', resetAndLoad);
  prevBtn.addEventListener('click', function () {
    if (pageIndex <= 0) return;
    pageIndex -= 1;
    offsetStack = offsetStack.slice(0, pageIndex + 1);
    loadPage();
  });
  nextBtn.addEventListener('click', function () {
    if (!hasMore) return;
    pageIndex += 1;
    offsetStack[pageIndex] = lastNextOffset;
    loadPage();
  });

  function resetAndLoad() {
    offsetStack = [0];
    pageIndex = 0;
    lastNextOffset = 0;
    hasMore = false;
    loadPage();
  }

  async function loadPage() {
    var params = new URLSearchParams();
    params.set('limit', String(pageSize));
    params.set('offset', String(offsetStack[pageIndex] || 0));
    if (statusFilter.value) params.set('status', statusFilter.value);
    var resp = await fetch('/admin/api/analytics/sync-summaries?' + params.toString());
    if (!resp.ok) {
      tbody.innerHTML = '<tr><td colspan="11">加载失败: ' + resp.status + '</td></tr>';
      return;
    }
    var data = ((await resp.json()).data) || {};
    var items = data.items || [];
    hasMore = !!data.has_more;
    lastNextOffset = data.next_offset || 0;
    tbody.innerHTML = '';
    if (!items.length) {
      tbody.innerHTML = '<tr><td colspan="11" class="hint">暂无同步摘要</td></tr>';
    }
    items.forEach(function (it) {
      var tr = document.createElement('tr');
      if (Number(it.id) === currentID) tr.className = 'selected';
      tr.innerHTML = '<td>' + esc(it.id) + '</td>' +
        '<td>' + statusBadge(it.status) + '</td>' +
        '<td>' + fmtTime(it.started_at) + '</td>' +
        '<td>' + fmtDuration(it.duration_ms) + '</td>' +
        '<td>' + esc(it.scanned_files) + '</td>' +
        '<td>' + esc(it.uploaded_files) + '</td>' +
        '<td>' + esc(it.failed_files) + '</td>' +
        '<td>' + fmtBytes(it.total_bytes) + '</td>' +
        '<td class="clip" title="' + esc(it.oss_prefix || '') + '">' + esc(it.oss_prefix || '') + '</td>' +
        '<td class="clip" title="' + esc(it.error_message || '') + '">' + esc(it.error_message || '') + '</td>' +
        '<td><button data-id="' + esc(it.id) + '">查看文件</button></td>';
      tbody.appendChild(tr);
    });
    tbody.querySelectorAll('button[data-id]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var id = Number(btn.dataset.id || 0);
        var found = items.find(function (it) { return Number(it.id) === id; });
        if (found) showDetail(found);
      });
    });
    prevBtn.disabled = pageIndex <= 0;
    nextBtn.disabled = !hasMore;
    pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
    totalHint.textContent = '共 ' + (data.total || 0) + ' 条';
  }

  function showDetail(item) {
    currentID = Number(item.id || 0);
    emptyDetail.classList.add('hidden');
    detailContent.classList.remove('hidden');
    setText('detailID', item.id);
    setText('detailJob', item.job_name || '');
    document.getElementById('detailStatus').innerHTML = statusBadge(item.status);
    setText('detailTime', fmtTime(item.started_at) + ' ~ ' + fmtTime(item.ended_at));

    var files = [];
    try {
      files = JSON.parse(item.files_json || '[]') || [];
    } catch (e) {
      files = [];
    }
    var fileList = document.getElementById('fileList');
    fileList.innerHTML = '';
    if (!files.length) {
      fileList.innerHTML = '<tr><td colspan="6" class="hint">无文件明细</td></tr>';
    }
    files.forEach(function (f) {
      var inputFiles = fmtInputFiles(f);
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + statusBadge(f.status) + '</td>' +
        '<td>' + fmtBytes(f.size_bytes) + '</td>' +
        '<td>' + esc(inputFileCount(f)) + '</td>' +
        '<td class="mono clip" title="' + esc(inputFiles) + '">' + esc(inputFiles || f.local_path || '') + '</td>' +
        '<td class="mono clip" title="' + esc(f.oss_key || '') + '">' + esc(f.oss_key || '') + '</td>' +
        '<td class="clip" title="' + esc(f.error || '') + '">' + esc(f.error || '') + '</td>';
      fileList.appendChild(tr);
    });
    loadPage();
  }

  function setText(id, value) {
    document.getElementById(id).textContent = value == null ? '' : String(value);
  }
  function statusBadge(status) {
    var s = String(status || '');
    return '<span class="badge badge-' + esc(s) + '">' + esc(s) + '</span>';
  }
  function fmtTime(s) {
    return esc((s || '').replace('T', ' ').slice(0, 19));
  }
  function fmtDuration(ms) {
    var n = Number(ms || 0);
    if (n < 1000) return n + ' ms';
    return (n / 1000).toFixed(2) + ' s';
  }
  function fmtBytes(n) {
    n = Number(n || 0);
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(2) + ' MB';
    return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB';
  }
  function inputFileCount(f) {
    if (f && f.input_file_count) return f.input_file_count;
    if (f && Array.isArray(f.input_files)) return f.input_files.length;
    return f && f.local_path ? 1 : 0;
  }
  function fmtInputFiles(f) {
    if (f && Array.isArray(f.input_files) && f.input_files.length) {
      return f.input_files.join('\n');
    }
    return '';
  }
  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  loadPage();
})();
