(function () {
  var endpoint = '/admin/api/feedback-correction-config';
  var root = document.getElementById('correctionConfigModal');
  var modal = new tabler.Modal(root, { backdrop: 'static' });
  var form = document.getElementById('correctionConfigForm');
  var rows = document.getElementById('correctionConfigRows');
  var table = document.getElementById('correctionConfigTable');
  var state = document.getElementById('correctionConfigState');
  var error = document.getElementById('correctionConfigError');
  var success = document.getElementById('correctionConfigSuccess');
  var add = document.getElementById('correctionConfigAdd');
  var reload = document.getElementById('correctionConfigReload');
  var save = document.getElementById('correctionConfigSave');
  var version = null;
  var busy = false;
  var dirty = false;

  function setDirty(value) {
    dirty = value;
    reload.textContent = value ? '重新加载（放弃修改）' : '重新加载';
    success.textContent = '';
  }

  function setBusy(value) {
    busy = value;
    form.querySelectorAll('input, button').forEach(function (element) { element.disabled = value; });
    add.disabled = value || version === null || rows.children.length >= 30;
    save.disabled = value || version === null;
  }

  function inputCell(row, field, type, value, label, maxLength) {
    var cell = document.createElement('td');
    var input = document.createElement('input');
    input.type = type;
    input.name = field;
    input.setAttribute('aria-label', label);
    input.className = type === 'checkbox' ? 'form-check-input' : 'form-control';
    if (type === 'checkbox') {
      input.checked = value;
    } else {
      input.value = value;
      if (maxLength) input.maxLength = maxLength;
    }
    if (field === 'name') input.required = true;
    if (type === 'number') {
      input.required = true;
      input.min = '0';
      input.max = '9999';
      input.step = '1';
    }
    cell.appendChild(input);
    row.appendChild(cell);
  }

  function appendRow(option) {
    var row = document.createElement('tr');
    row.dataset.optionId = option.id || '';
    inputCell(row, 'name', 'text', option.name || '', '中文名称', 24);
    inputCell(row, 'name_en', 'text', option.name_en || '', '英文名称', 64);
    inputCell(row, 'sort_order', 'number', option.sort_order, '排序');
    inputCell(row, 'enabled', 'checkbox', option.enabled, '启用');
    var actions = document.createElement('td');
    if (!option.id) {
      var remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn btn-sm btn-outline-danger';
      remove.textContent = '移除';
      remove.addEventListener('click', function () {
        row.remove();
        setDirty(true);
        setBusy(false);
      });
      actions.appendChild(remove);
    }
    row.appendChild(actions);
    rows.appendChild(row);
  }

  function render(config) {
    version = config.version;
    rows.replaceChildren();
    config.options.forEach(appendRow);
    table.classList.remove('hidden');
    state.textContent = '';
    setDirty(false);
  }

  async function request(options) {
    var response = await fetch(endpoint, options);
    if (response.status === 401) {
      window.location.href = '/admin/login.html';
      throw new Error('请重新登录');
    }
    var json = await response.json();
    if (!response.ok) throw new Error(json.error || json.message || '请求失败');
    if (!json.data || !Array.isArray(json.data.options) || !Number.isSafeInteger(json.data.version)) {
      throw new Error('配置数据异常，请重新加载');
    }
    return json.data;
  }

  async function load() {
    if (busy) return;
    error.textContent = '';
    success.textContent = '';
    state.textContent = '加载中…';
    setBusy(true);
    try {
      render(await request({ cache: 'no-store' }));
    } catch (err) {
      error.textContent = err.message || '加载失败，请重试';
      state.textContent = '';
    } finally {
      setBusy(false);
    }
  }

  document.getElementById('correctionConfigBtn').addEventListener('click', function () {
    modal.show();
    if (!dirty) load();
  });
  root.addEventListener('hide.bs.modal', function (event) {
    if (busy) event.preventDefault();
  });
  rows.addEventListener('input', function () { setDirty(true); });
  rows.addEventListener('change', function () { setDirty(true); });
  reload.addEventListener('click', load);
  add.addEventListener('click', function () {
    if (busy || rows.children.length >= 30) return;
    var maxOrder = Array.from(rows.querySelectorAll('[name=sort_order]')).reduce(function (max, input) {
      return Math.max(max, Number(input.value) || 0);
    }, 0);
    appendRow({ enabled: true, sort_order: Math.min(9999, maxOrder + 10) });
    setDirty(true);
    setBusy(false);
    rows.lastElementChild.querySelector('input').focus();
  });
  form.addEventListener('submit', async function (event) {
    event.preventDefault();
    if (busy || version === null || !form.reportValidity()) return;
    var options = Array.from(rows.children).map(function (row) {
      return {
        id: row.dataset.optionId,
        name: row.querySelector('[name=name]').value.trim(),
        name_en: row.querySelector('[name=name_en]').value.trim(),
        sort_order: Number(row.querySelector('[name=sort_order]').value),
        enabled: row.querySelector('[name=enabled]').checked
      };
    });
    error.textContent = '';
    success.textContent = '';
    setBusy(true);
    try {
      render(await request({
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: version, options: options })
      }));
      success.textContent = '已保存';
    } catch (err) {
      // Keep the edited rows intact, including when another administrator saved a newer version.
      error.textContent = err.message || '保存失败，请重试';
    } finally {
      setBusy(false);
    }
  });
})();
