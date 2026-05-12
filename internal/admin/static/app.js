(function () {
  // 当前会话信息
  const userNameEl = document.getElementById('userName');

  // -- 登录态校验 --
  fetch('/admin/api/me').then(function (r) {
    if (!r.ok) {
      window.location.href = '/admin/login.html';
      return null;
    }
    return r.json();
  }).then(function (j) {
    if (j && j.data) userNameEl.textContent = '👤 ' + j.data.username;
  });

  // -- 退出 --
  document.getElementById('logoutBtn').addEventListener('click', async function () {
    await fetch('/admin/api/logout', { method: 'POST' });
    window.location.href = '/admin/login.html';
  });

  // ---------------- 本机上传 ----------------
  // 安装包直接 multipart 上传到服务端 <staticRoot>/release/<platform>/，
  // 服务端返回的 url 形如 "/api/v1/static/release/android/<filename>"，
  // 直接填入 package_url 即可。
  const apkFile = document.getElementById('apkFile');
  const uploadBtn = document.getElementById('uploadBtn');
  const uploadHint = document.getElementById('uploadHint');
  const pkgURLInput = document.querySelector('input[name=package_url]');
  const pkgSizeInput = document.querySelector('input[name=package_size]');
  const platformSelect = document.querySelector('select[name=platform]');

  uploadBtn.addEventListener('click', async function () {
    const file = apkFile.files[0];
    if (!file) {
      uploadHint.textContent = '请先选择安装包文件';
      return;
    }
    const platform = (platformSelect && platformSelect.value) || 'android';
    uploadHint.textContent = '正在上传 (' + (file.size / 1024 / 1024).toFixed(2) + ' MB) ...';
    try {
      const fd = new FormData();
      fd.append('platform', platform);
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
      uploadHint.textContent = '上传成功';
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
  const filterPlatform = document.getElementById('filterPlatform');
  const filterStatus = document.getElementById('filterStatus');
  document.getElementById('refreshBtn').addEventListener('click', loadList);
  filterPlatform.addEventListener('change', loadList);
  filterStatus.addEventListener('change', loadList);

  async function loadList() {
    const params = new URLSearchParams();
    if (filterPlatform.value) params.set('platform', filterPlatform.value);
    if (filterStatus.value) params.set('status', filterStatus.value);
    const resp = await fetch('/admin/api/releases?' + params.toString());
    if (!resp.ok) return;
    const items = ((await resp.json()).data || {}).items || [];
    const tbody = document.getElementById('releaseList');
    tbody.innerHTML = '';
    items.forEach(function (it) {
      const tr = document.createElement('tr');
      tr.innerHTML = '<td>' + it.id + '</td>' +
        '<td>' + esc(it.platform) + '</td>' +
        '<td>' + esc(it.version_name) + '</td>' +
        '<td>' + it.version_code + '</td>' +
        '<td>' + it.min_supported_version_code + '</td>' +
        '<td>' + (it.force_update ? '是' : '否') + '</td>' +
        '<td>' + esc(it.status) + '</td>' +
        '<td>' + esc(it.operator_name || '') + '</td>' +
        '<td>' + esc((it.updated_at || '').replace('T', ' ').slice(0, 19)) + '</td>' +
        '<td class="url" title="' + esc(it.package_url) + '"><a href="' + esc(it.package_url) + '" target="_blank">' + esc(it.package_url) + '</a></td>' +
        '<td><button class="danger" data-id="' + it.id + '">删除</button></td>';
      tbody.appendChild(tr);
    });
    tbody.querySelectorAll('button.danger').forEach(function (b) {
      b.addEventListener('click', async function () {
        if (!confirm('确认删除版本 #' + b.dataset.id + ' ?')) return;
        const r = await fetch('/admin/api/releases/' + b.dataset.id, { method: 'DELETE' });
        if (!r.ok) { alert('删除失败'); return; }
        loadList();
      });
    });
  }

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  loadList();
})();
