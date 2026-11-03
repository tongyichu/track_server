(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  let catalog = null;
  let selectedId = '';
  let activeCountryId = '';
  let dirty = false;
  let persistedIds = new Set();
  let expandedIds = new Set();
  const fieldMap = {
    regionName: 'name', regionNameEn: 'name_en',
    regionCountryCode: 'country_code', regionSlogan: 'slogan', regionDescription: 'description'
  };

  function externalValue(node, provider, type) {
    return (node.external_ids || []).find(item => item.provider === provider && item.type === type)?.value || '';
  }

  function setExternalValue(node, provider, type, value) {
    node.external_ids = (node.external_ids || []).filter(item => item.provider !== provider || item.type !== type);
    if (value && !(provider === 'amap' && type === 'adcode' && node.code_system === 'cn_adcode' && node.code === value)) {
      node.external_ids.push({ provider, type, value });
    }
  }

  function status(message, error) {
    $('regionStatus').textContent = message;
    $('regionStatus').className = error ? 'text-danger' : 'text-secondary';
  }

  async function api(path, options) {
    const response = await fetch('/admin/api/regions/' + path, options);
    let body;
    try { body = await response.json(); } catch (_) { throw new Error('服务器返回无效内容'); }
    if (!response.ok || body.code !== 0) throw new Error(body.error || '操作失败');
    return body.data;
  }

  function current() { return catalog && catalog.nodes.find(node => node.id === selectedId); }
  function children(parentId) {
    return catalog.nodes.filter(node => node.parent_id === parentId)
      .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0) || a.name.localeCompare(b.name, 'zh-CN'));
  }
  function ancestors(id) {
    const result = new Set();
    let node = catalog.nodes.find(item => item.id === id);
    while (node && node.parent_id) {
      if (result.has(node.parent_id)) break;
      result.add(node.parent_id);
      node = catalog.nodes.find(item => item.id === node.parent_id);
    }
    return result;
  }

  function countryOf(node) {
    const seen = new Set();
    while (node && !seen.has(node.id)) {
      if (!node.parent_id) return node.level === 'country' ? node : null;
      seen.add(node.id);
      node = catalog.nodes.find(item => item.id === node.parent_id);
    }
    return null;
  }

  function renderCountryTabs() {
    const root = $('regionCountryTabs');
    root.replaceChildren();
    if (!catalog) return;
    children('').filter(node => node.level === 'country').forEach(node => {
      const tab = document.createElement('button');
      tab.type = 'button';
      tab.className = 'region-country-tab' + (node.id === activeCountryId ? ' active' : '') + (!node.active ? ' archived' : '');
      tab.setAttribute('role', 'tab');
      tab.setAttribute('aria-selected', node.id === activeCountryId ? 'true' : 'false');
      tab.setAttribute('aria-controls', 'regionTree');
      const title = document.createElement('span');
      title.textContent = node.name;
      const code = document.createElement('small');
      code.textContent = node.country_code || node.code || '';
      tab.append(title, code);
      tab.addEventListener('click', () => {
        $('regionSearch').value = '';
        select(node.id);
        root.querySelector('[aria-selected="true"]')?.focus();
      });
      root.appendChild(tab);
    });
  }

  function renderTree() {
    const root = $('regionTree');
    root.replaceChildren();
    if (!catalog || !activeCountryId) return;
    const query = $('regionSearch').value.trim().toLowerCase();
    const visible = new Set();
    if (query) catalog.nodes.forEach(node => {
      if (countryOf(node)?.id === activeCountryId &&
          (node.name + ' ' + (node.name_en || '') + ' ' + (node.code || '') + ' ' +
            (node.external_ids || []).map(item => item.value).join(' ')).toLowerCase().includes(query)) {
        visible.add(node.id);
        ancestors(node.id).forEach(id => visible.add(id));
      }
    });
    function add(parentId, list) {
      children(parentId).forEach(node => {
        if (query && !visible.has(node.id)) return;
        const childNodes = children(node.id).filter(item => !query || visible.has(item.id));
        const expanded = childNodes.length > 0 && (query ? true : expandedIds.has(node.id));
        const item = document.createElement('li');
        item.className = 'region-tree-node' + (!node.active ? ' archived' : '');
        item.setAttribute('role', 'none');
        const row = document.createElement('div');
        row.className = 'region-tree-row';
        if (childNodes.length) {
          const toggle = document.createElement('button');
          toggle.type = 'button';
          toggle.className = 'region-tree-toggle';
          toggle.textContent = expanded ? '−' : '+';
          toggle.setAttribute('aria-label', (expanded ? '折叠' : '展开') + node.name);
          toggle.setAttribute('aria-expanded', expanded ? 'true' : 'false');
          toggle.disabled = !!query;
          toggle.addEventListener('click', () => {
            if (expandedIds.has(node.id)) expandedIds.delete(node.id);
            else expandedIds.add(node.id);
            renderTree();
          });
          row.appendChild(toggle);
        } else {
          const spacer = document.createElement('span');
          spacer.className = 'region-tree-toggle-spacer';
          row.appendChild(spacer);
        }
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'region-tree-item' + (node.id === selectedId ? ' selected' : '');
        button.setAttribute('role', 'treeitem');
        button.setAttribute('aria-selected', node.id === selectedId ? 'true' : 'false');
        if (childNodes.length) button.setAttribute('aria-expanded', expanded ? 'true' : 'false');
        const label = document.createElement('span');
        label.textContent = node.name;
        button.appendChild(label);
        button.addEventListener('click', () => select(node.id));
        row.appendChild(button);
        item.appendChild(row);
        if (expanded) {
          const nested = document.createElement('ul');
          nested.className = 'region-tree-children';
          nested.setAttribute('role', 'group');
          add(node.id, nested);
          item.appendChild(nested);
        }
        list.appendChild(item);
      });
    }
    const tree = document.createElement('ul');
    tree.className = 'region-tree-list';
    add(activeCountryId, tree);
    if (tree.childElementCount) root.appendChild(tree);
    else root.textContent = query ? '没有匹配的地区' : '当前国家还没有下级地区';
  }

  function preview(url) {
    const image = $('regionCoverPreview');
    if (!url) { image.hidden = true; image.removeAttribute('src'); return; }
    image.hidden = false;
    image.src = url.startsWith('/api/v1/static/regions/')
      ? '/admin/api/static/regions/' + url.split('/').pop() : url;
  }

  function select(id) {
    selectedId = id;
    const node = current();
    if (node) {
      const country = countryOf(node);
      if (country) activeCountryId = country.id;
      ancestors(node.id).forEach(parentId => { if (parentId !== activeCountryId) expandedIds.add(parentId); });
    }
    renderCountryTabs();
    $('regionEmpty').hidden = !!node;
    $('regionForm').hidden = !node;
    $('regionAddChild').disabled = !node || !node.active || !(window.AdminAccess && AdminAccess.can('regions', true));
    if (!node) { renderTree(); return; }
    $('regionEditorTitle').textContent = node.name || '新增地区';
    $('regionId').textContent = '内部 ID：' + (node.region_id || '保存后分配');
    Object.entries(fieldMap).forEach(([id, key]) => { $(id).value = node[key] || ''; });
    $('regionAmapAdcode').value = externalValue(node, 'amap', 'adcode') || (node.code_system === 'cn_adcode' ? node.code || '' : '');
    $('regionGooglePlaceId').value = externalValue(node, 'google', 'place_id');
    const countryRoot = node.level === 'country' && !node.parent_id;
    $('regionCountryCodeLabel').textContent = countryRoot ? '国家缩写（两位）' : '所属国家代码';
    $('regionCountryCode').required = countryRoot && node.active;
    $('regionCountryCode').disabled = !countryRoot || persistedIds.has(node.id);
    $('regionContinentField').hidden = node.level !== 'country';
    $('regionContinent').value = node.continent || '';
    $('regionLatitude').value = node.latitude == null ? '' : String(node.latitude);
    $('regionLongitude').value = node.longitude == null ? '' : String(node.longitude);
    $('regionSelectable').checked = !!node.selectable;
    $('regionDeleteDraft').hidden = persistedIds.has(node.id);
    preview(node.cover_url);
    renderTree();
  }

  function syncForm() {
    const node = current();
    if (!node) return;
    Object.entries(fieldMap).forEach(([id, key]) => { node[key] = $(id).value.trim(); });
    setExternalValue(node, 'amap', 'adcode', $('regionAmapAdcode').value.trim());
    setExternalValue(node, 'google', 'place_id', $('regionGooglePlaceId').value.trim());
    node.country_code = node.country_code.toUpperCase();
    if (node.level === 'country' && !persistedIds.has(node.id)) {
      node.code = node.country_code;
    }
    if (node.level === 'country') node.continent = $('regionContinent').value;
    if (node.code && !node.code_system) {
      const countryCode = node.country_code.toUpperCase();
      node.code_system = node.level === 'country' ? 'iso3166-1' : countryCode === 'CN' && /^\d{6}$/.test(node.code) ? 'cn_adcode' : 'local:' + (countryCode || countryOf(node)?.id || 'unknown');
    }
    node.latitude = $('regionLatitude').value === '' ? null : Number($('regionLatitude').value);
    node.longitude = $('regionLongitude').value === '' ? null : Number($('regionLongitude').value);
    node.selectable = $('regionSelectable').checked;
    $('regionEditorTitle').textContent = node.name || '新增地区';
    dirty = true;
    status('有未保存修改');
    preview(node.cover_url);
    const country = countryOf(node);
    if (country) activeCountryId = country.id;
    renderCountryTabs();
    renderTree();
  }

  function addNode(parent) {
    if (!catalog) return;
    const country = !parent;
    let id;
    do {
      const values = crypto.getRandomValues(new Uint8Array(5));
      id = 'R' + Array.from(values, value => (value % 36).toString(36).toUpperCase()).join('');
    } while (catalog.nodes.some(node => node.id === id));
    const siblings = children(parent ? parent.id : '');
    const china = parent && (parent.country_code === 'CN' || parent.id === 'country:CN');
    const node = {
      id, parent_id: parent ? parent.id : '', name: country ? '新国家' : '新地区',
      level: country ? 'country' : china && parent.level === 'country' ? 'province' : china && parent.level === 'city' ? 'district' : 'city',
      code: '', code_system: '',
      country_code: parent ? parent.country_code || '' : '', continent: parent ? parent.continent || '' : '',
      selectable: false, active: true, sort_order: siblings.reduce((max, item) => Math.max(max, item.sort_order || 0), -1) + 1
    };
    catalog.nodes.push(node);
    if (parent) expandedIds.add(parent.id);
    dirty = true;
    select(id);
    $('regionName').focus();
    status('已新增节点，请填写资料并保存');
  }

  async function load() {
    status('正在加载地区目录…');
    try {
      catalog = await api('catalog');
      persistedIds = new Set(catalog.nodes.map(node => node.id));
      expandedIds = new Set();
      dirty = false;
      $('regionVersion').textContent = '目录版本 ' + catalog.version + ' · ' + catalog.nodes.length + ' 个节点';
      select(catalog.nodes.some(node => node.id === selectedId) ? selectedId : children('').find(node => node.level === 'country')?.id || '');
      status('地区目录已加载');
    } catch (error) { status(error.message, true); }
  }

  $('regionSearch').addEventListener('input', renderTree);
  $('regionForm').addEventListener('input', syncForm);
  $('regionForm').addEventListener('change', syncForm);
  $('regionAddCountry').addEventListener('click', () => addNode(null));
  $('regionAddChild').addEventListener('click', () => addNode(current()));
  $('regionDeleteDraft').addEventListener('click', () => {
    const node = current();
    if (!node || persistedIds.has(node.id)) return;
    const removed = new Set([node.id]);
    catalog.nodes.forEach(item => { if (ancestors(item.id).has(node.id)) removed.add(item.id); });
    catalog.nodes = catalog.nodes.filter(item => !removed.has(item.id));
    dirty = true;
    select(catalog.nodes.some(item => item.id === activeCountryId) ? activeCountryId : children('').find(item => item.level === 'country')?.id || '');
    status('未保存节点及下级已删除');
  });
  $('regionReload').addEventListener('click', load);
  $('regionSave').addEventListener('click', async () => {
    if (!catalog || !dirty) return;
    $('regionSave').disabled = true;
    status('正在保存…');
    try {
      catalog = await api('catalog', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(catalog) });
      persistedIds = new Set(catalog.nodes.map(node => node.id));
      dirty = false;
      $('regionVersion').textContent = '目录版本 ' + catalog.version + ' · ' + catalog.nodes.length + ' 个节点';
      select(selectedId);
      status('保存成功，客户端下次刷新目录时会读取新版本');
    } catch (error) { status(error.message, true); }
    finally { $('regionSave').disabled = false; }
  });
  $('regionImageFile').addEventListener('change', async () => {
    const file = $('regionImageFile').files[0];
    if (!file) return;
    if (file.size > 8 * 1024 * 1024) { status('图片不能超过 8 MiB', true); return; }
    const nodeId = selectedId;
    const form = new FormData();
    form.append('image', file);
    status('正在上传图片…');
    try {
      const result = await api('images', { method: 'POST', body: form });
      const node = catalog.nodes.find(item => item.id === nodeId);
      if (node) node.cover_url = result.cover_url;
      if (selectedId === nodeId) preview(result.cover_url);
      dirty = true;
      status('图片已上传，请保存地区资料');
    } catch (error) { status(error.message, true); }
    $('regionImageFile').value = '';
  });
  window.addEventListener('beforeunload', event => { if (dirty) { event.preventDefault(); event.returnValue = ''; } });
  load();
})();
