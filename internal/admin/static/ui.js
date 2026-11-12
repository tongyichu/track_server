(function () {
  const pageMeta = {
    '/admin/realtime.html': ['实时通信', '在线私信关系、同行房间与连接诊断'],
    '/admin/index.html': ['发布管理', '安装包上传、版本发布与历史版本管理'],
    '/admin/users.html': ['用户管理', '查看用户资料并管理账号限制'],
    '/admin/tracks.html': ['轨迹管理', '查看、修正或删除用户轨迹'],
    '/admin/scenic_spots.html': ['景点管理', '维护自有景点资料、地图来源与轨迹归属'],
    '/admin/track_submissions.html': ['投稿审核', '审核用户提交的结构化路线资料'],
    '/admin/route_groups.html': ['地图模式', '维护地图模式中的路线组、代表轨迹和路线介绍'],
    '/admin/companions.html': ['与友同行', '查看同行房间、成员与活动记录'],
    '/admin/companion_detail.html': ['同行详情', '查看成员、实时位置和弹幕历史'],
    '/admin/feedbacks.html': ['反馈与纠错', '核查轨迹问题，回复处理结果'],
    '/admin/analytics.html': ['埋点同步', '查看埋点文件同步状态和明细']
  };

  function toast(message, tone) {
    let host = document.getElementById('adminToastHost');
    if (!host) {
      host = document.createElement('div');
      host.id = 'adminToastHost';
      host.className = 'toast-container position-fixed top-0 end-0 p-3';
      document.body.appendChild(host);
    }
    const element = document.createElement('div');
    element.className = 'toast border-0';
    element.setAttribute('role', 'status');
    const body = document.createElement('div');
    body.className = 'toast-body d-flex align-items-center gap-2';
    const marker = document.createElement('span');
    marker.className = 'status-dot status-dot-animated bg-' + (tone === 'danger' ? 'red' : tone === 'success' ? 'green' : 'azure');
    const text = document.createElement('span');
    text.textContent = String(message || '');
    body.append(marker, text);
    element.appendChild(body);
    host.appendChild(element);
    const instance = new tabler.Toast(element, { delay: 3600 });
    element.addEventListener('hidden.bs.toast', function () { element.remove(); });
    instance.show();
  }

  function createModal(options) {
    const root = document.createElement('div');
    root.className = 'modal modal-blur fade';
    root.tabIndex = -1;
    root.innerHTML = '<div class="modal-dialog modal-dialog-centered' + (options.wide ? ' modal-dialog-scrollable modal-xl' : '') + '"><div class="modal-content"><div class="modal-header"><h5 class="modal-title"></h5><button type="button" class="btn-close" aria-label="关闭"></button></div><div class="modal-body"></div><div class="modal-footer"></div></div></div>';
    root.querySelector('.modal-title').textContent = options.title || '提示';
    document.body.appendChild(root);
    return root;
  }

  function confirmDialog(options) {
    if (typeof options === 'string') options = { message: options };
    options = options || {};
    return new Promise(function (resolve) {
      const root = createModal(options);
      const body = root.querySelector('.modal-body');
      body.textContent = options.message || '';
      const footer = root.querySelector('.modal-footer');
      const cancel = document.createElement('button');
      cancel.type = 'button';
      cancel.className = 'btn btn-outline-secondary';
      cancel.textContent = options.cancelText || '取消';
      const submit = document.createElement('button');
      submit.type = 'button';
      submit.className = 'btn ' + (options.danger ? 'btn-danger' : 'btn-primary');
      submit.textContent = options.confirmText || '确认';
      footer.append(cancel, submit);
      const modal = new tabler.Modal(root, { backdrop: 'static' });
      let settled = false;
      function finish(value) {
        if (settled) return;
        settled = true;
        resolve(value);
        modal.hide();
      }
      cancel.addEventListener('click', function () { finish(false); });
      root.querySelector('.btn-close').addEventListener('click', function () { finish(false); });
      submit.addEventListener('click', function () { finish(true); });
      root.addEventListener('hidden.bs.modal', function () { if (!settled) resolve(false); root.remove(); });
      modal.show();
    });
  }

  const openingTasks = new Map();
  function runOnce(key, action) {
    if (openingTasks.has(key)) return openingTasks.get(key);
    // 先登记任务，再执行异步入口，重复点击复用整个工作流，避免重复保存。
    const task = Promise.resolve().then(action);
    openingTasks.set(key, task);
    const release = function () { if (openingTasks.get(key) === task) openingTasks.delete(key); };
    task.then(release, release);
    return task;
  }

  function formDialog(options) {
    options = options || {};
    return new Promise(function (resolve) {
      const root = createModal(options);
      const body = root.querySelector('.modal-body');
      const footer = root.querySelector('.modal-footer');
      const close = root.querySelector('.btn-close');
      const modal = new tabler.Modal(root, { backdrop: 'static' });
      const controller = new AbortController();
      let config = options, settled = false, hidden = false, loading = false, pendingValue = null, form = null;
      function finish(value) {
        if (settled || hidden) return;
        settled = true; pendingValue = value;
        controller.abort();
        if (!config.waitForHidden) resolve(value);
        modal.hide();
      }
      function focusField() {
        if (settled || hidden || !form) return;
        const first = form.querySelector('input:not(:disabled):not([readonly]), textarea:not(:disabled):not([readonly]), select:not(:disabled)');
        if (first) first.focus();
      }
      close.addEventListener('click', function () { finish(null); });
      root.addEventListener('shown.bs.modal', function () {
        // Bootstrap 在入场动画中忽略 hide；补齐快速关闭，避免留下无响应的加载框。
        if (settled) modal.hide(); else focusField();
      });
      root.addEventListener('hide.bs.modal', function () { controller.abort(); });
      root.addEventListener('hidden.bs.modal', function () {
        hidden = true; controller.abort();
        try { if (typeof config.onClosed === 'function') config.onClosed(); }
        finally {
          if (!settled) resolve(null); else if (config.waitForHidden) resolve(pendingValue);
          modal.dispose(); root.remove();
        }
      });
      function render(loaded) {
        if (settled || hidden || controller.signal.aborted) return;
        config = Object.assign({}, options, loaded);
        root.querySelector('.modal-title').textContent = config.title || '提示';
        if (config.className) root.classList.add(config.className);
        body.removeAttribute('aria-busy'); body.replaceChildren(); footer.replaceChildren();
        form = document.createElement('form'); form.className = 'admin-dialog-form';
        if (config.beforeFields) body.appendChild(config.beforeFields);
        (config.fields || []).forEach(function (field) {
          const group = document.createElement('label'); group.className = 'form-label'; group.textContent = field.label || field.name;
          const input = document.createElement(field.type === 'textarea' ? 'textarea' : (field.type === 'select' ? 'select' : 'input'));
          input.className = 'form-control'; input.name = field.name;
          if (field.full) group.classList.add('full');
          if (field.type && field.type !== 'textarea' && field.type !== 'select') input.type = field.type;
          if (field.type === 'select') (field.options || []).forEach(function (option) {
            const item = document.createElement('option'); item.value = String(option.value); item.textContent = String(option.label); input.appendChild(item);
          });
          if (field.value != null) input.value = field.value;
          if (field.placeholder) input.placeholder = field.placeholder;
          if (field.required) input.required = true;
          if (field.maxLength) input.maxLength = field.maxLength;
          if (field.min != null) input.min = String(field.min);
          if (field.step != null) input.step = String(field.step);
          if (field.readOnly) input.readOnly = true;
          if (config.readOnly) input.disabled = true;
          if (typeof field.onChange === 'function') input.addEventListener('change', function () { field.onChange(input.value); });
          group.appendChild(input); form.appendChild(group);
        });
        body.appendChild(form);
        if (config.afterFields) body.appendChild(config.afterFields);
        const cancel = document.createElement('button'); cancel.type = 'button';
        cancel.className = config.cancelDanger ? 'btn btn-outline-danger' : 'btn btn-outline-secondary'; cancel.textContent = config.cancelText || '取消';
        const submit = document.createElement('button'); submit.type = 'submit'; submit.className = 'btn btn-primary'; submit.textContent = config.confirmText || '保存';
        if (config.readOnly) submit.hidden = true;
        footer.append(cancel, submit);
        let cancelling = false;
        cancel.addEventListener('click', async function () {
          if (settled || cancelling) return;
          if (typeof config.onCancel !== 'function') { finish(null); return; }
          cancelling = true; cancel.disabled = true;
          try { const action = await config.onCancel(); if (action) finish({ _action: action }); }
          catch (error) { window.AdminUI.alert(error.message || String(error), 'danger'); }
          finally { cancelling = false; cancel.disabled = false; }
        });
        form.addEventListener('submit', function (event) {
          event.preventDefault();
          if (settled || cancelling || config.readOnly || !form.reportValidity()) return;
          const values = {};
          new FormData(form).forEach(function (value, key) { values[key] = String(value); });
          finish(values);
        });
        submit.addEventListener('click', function () { form.requestSubmit(); });
        if (typeof config.onReady === 'function') config.onReady({ root: root, form: form, signal: controller.signal });
        if (root.classList.contains('show')) focusField();
      }
      async function load() {
        if (loading || settled || hidden || controller.signal.aborted) return;
        loading = true;
        body.setAttribute('aria-busy', 'true'); body.replaceChildren(); footer.replaceChildren();
        const status = document.createElement('div'); status.className = 'd-flex align-items-center gap-2 py-4'; status.setAttribute('role', 'status');
        const spinner = document.createElement('span'); spinner.className = 'spinner-border spinner-border-sm'; spinner.setAttribute('aria-hidden', 'true');
        const message = document.createElement('span'); message.textContent = '正在加载详情…'; status.append(spinner, message); body.appendChild(status);
        const cancel = document.createElement('button'); cancel.type = 'button'; cancel.className = 'btn btn-outline-secondary'; cancel.textContent = '关闭';
        cancel.addEventListener('click', function () { finish(null); }); footer.appendChild(cancel);
        try { render(await options.load(controller.signal)); }
        catch (error) {
          if (settled || hidden || controller.signal.aborted) return;
          body.removeAttribute('aria-busy'); status.replaceChildren();
          message.textContent = error.message || '详情加载失败'; status.appendChild(message);
          const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-primary'; retry.textContent = '重试';
          retry.addEventListener('click', load); footer.prepend(retry);
        } finally { loading = false; }
      }
      if (typeof options.load === 'function') load(); else render(options);
      modal.show();
    });
  }

  window.AdminUI = { alert: toast, confirm: confirmDialog, form: formDialog, runOnce: runOnce };

  let imageViewer;
  function closeImageViewer() {
    if (!imageViewer) return;
    const viewer = imageViewer;
    imageViewer = null;
    if (viewer.open) viewer.close();
    viewer.remove();
    document.body.classList.remove('admin-image-viewer-open');
  }
  function openImageViewer(image, source) {
    closeImageViewer();
    const overlay = document.createElement('dialog');
    overlay.className = 'admin-image-viewer';
    overlay.setAttribute('aria-label', '图片预览');
    const enlarged = document.createElement('img');
    enlarged.src = source;
    enlarged.alt = image.alt || '';
    enlarged.draggable = false;
    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'admin-image-viewer-close';
    close.setAttribute('aria-label', '关闭图片预览');
    close.textContent = '×';
    overlay.append(enlarged, close);
    if (image.alt && image.alt.trim()) {
      const caption = document.createElement('div');
      caption.className = 'admin-image-viewer-caption';
      caption.textContent = image.alt.trim();
      overlay.appendChild(caption);
    }
    overlay.addEventListener('click', event => { if (event.target === overlay || event.target === close) closeImageViewer(); });
    document.body.appendChild(overlay);
    imageViewer = overlay;
    document.body.classList.add('admin-image-viewer-open');
    overlay.addEventListener('cancel', event => { event.preventDefault(); closeImageViewer(); });
    overlay.showModal();
    close.focus();
  }
  document.addEventListener('click', event => {
    if (!(event.target instanceof HTMLImageElement)) return;
    const image = event.target;
    if (image.closest('[data-no-preview], .admin-image-viewer') || image.classList.contains('nav-icon') || image.id === 'coordinateMapImage') return;
    const link = image.closest('a[href]');
    const source = image.dataset.fullSrc || (link ? link.href : '') || image.currentSrc || image.src;
    if (!source) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    openImageViewer(image, source);
  }, true);

  function addPageHeading() {
    const meta = pageMeta[window.location.pathname];
    const main = document.querySelector('main.container');
    if (!meta || !main || main.querySelector(':scope > .page-heading')) return;
    const heading = document.createElement('div');
    heading.className = 'page-heading admin-page-heading';
    heading.innerHTML = '<div><h1></h1><p></p></div>';
    heading.querySelector('h1').textContent = meta[0];
    heading.querySelector('p').textContent = meta[1];
    main.insertBefore(heading, main.firstChild);
  }

  function enhanceCard(panel) {
    if (panel.dataset.tablerCard === 'true') return;
    panel.dataset.tablerCard = 'true';
    panel.classList.add('card');
    if (panel.closest('.dashboard-page')) return;
    const title = Array.from(panel.children).find(function (child) {
      return child.tagName === 'H2' && !child.classList.contains('subhead');
    });
    if (!title) {
      panel.classList.add('card-body');
      return;
    }
    const header = document.createElement('div');
    header.className = 'card-header';
    title.parentNode.insertBefore(header, title);
    title.classList.add('card-title');
    header.appendChild(title);
    const body = document.createElement('div');
    body.className = 'card-body';
    while (header.nextSibling) body.appendChild(header.nextSibling);
    panel.appendChild(body);
  }

  function enhanceButton(button) {
    if (button.matches('.scenic-photo-picker, .region-photo-picker, .admin-photo-picker, .admin-photo-remove, .admin-photo-download')) return;
    if (button.hasAttribute('data-period') || button.hasAttribute('data-stat-period') || button.classList.contains('btn-close') || button.matches('.dropdown-item, [data-bs-toggle="dropdown"]')) return;
    if (button.closest('.region-country-bar, .region-tree, .admin-region-field, .admin-region-popup')) return;
    button.classList.add('btn');
    if (Array.from(button.classList).some(function (name) { return /^btn-(primary|secondary|success|danger|warning|info|light|dark|outline)/.test(name); })) return;
    const id = button.id || '';
    const isDanger = button.classList.contains('danger') || id === 'rejectBtn';
    const isSuccess = id === 'approveBtn';
    const isSecondary = /^(prevBtn|nextBtn|logoutBtn)$/.test(id) || button.classList.contains('quiet-button');
    const inTable = Boolean(button.closest('table'));
    if (isDanger) button.classList.add(inTable ? 'btn-outline-danger' : 'btn-danger');
    else if (isSuccess) button.classList.add('btn-success');
    else if (isSecondary) button.classList.add('btn-outline-secondary');
    else if (inTable) button.classList.add('btn-sm', 'btn-outline-primary');
    else button.classList.add('btn-primary');
  }

  function enhanceField(field) {
    if (field.matches('input[type="checkbox"], input[type="radio"]')) {
      field.classList.add('form-check-input');
    } else if (field.tagName === 'SELECT') {
      field.classList.add('form-select');
    } else {
      field.classList.add('form-control');
    }
  }

  function enhanceBadge(badge) {
    const variants = [
      ['badge-pending', 'bg-yellow-lt', 'text-yellow'],
      ['badge-processing', 'bg-azure-lt', 'text-azure'],
      ['badge-resolved', 'bg-green-lt', 'text-green'],
      ['badge-success', 'bg-green-lt', 'text-green'],
      ['badge-partial', 'bg-yellow-lt', 'text-yellow'],
      ['badge-failed', 'bg-red-lt', 'text-red'],
      ['badge-ignored', 'bg-secondary-lt', 'text-secondary']
    ];
    variants.forEach(function (variant) {
      if (badge.classList.contains(variant[0])) badge.classList.add(variant[1], variant[2]);
    });
  }

  function enhanceTable(table) {
    table.classList.add('table', 'table-vcenter', 'table-hover', 'admin-card-table');
    const headers = Array.from(table.querySelectorAll('thead th')).map(function (th) {
      return th.textContent.trim();
    });
    table.querySelectorAll('tbody tr').forEach(function (row) {
      const cells = Array.from(row.children);
      if (cells.length === 1 && cells[0].hasAttribute('colspan')) {
        row.classList.add('admin-empty-row');
        return;
      }
      cells.forEach(function (cell, index) {
        if (!cell.dataset.label) cell.dataset.label = headers[index] || '';
        if (/状态$/.test(cell.dataset.label) && !cell.children.length && cell.textContent.trim()) {
          const value = cell.textContent.trim();
          const badge = document.createElement('span');
          badge.className = 'badge ' + statusBadgeClass(value);
          badge.textContent = value;
          cell.textContent = '';
          cell.appendChild(badge);
        }
      });
    });
  }

  function statusBadgeClass(value) {
    const normalized = value.toLowerCase();
    if (/正常|通过|完成|成功|发布|approved|success|resolved|active|completed|published/.test(normalized)) return 'bg-green-lt text-green';
    if (/待|草稿|pending|partial|draft/.test(normalized)) return 'bg-yellow-lt text-yellow';
    if (/失败|驳回|删除|failed|rejected|deleted|invalidated/.test(normalized)) return 'bg-red-lt text-red';
    if (/进行|处理中|running|processing/.test(normalized)) return 'bg-azure-lt text-azure';
    return 'bg-secondary-lt text-secondary';
  }

  function enhance(root) {
    const scope = root.nodeType === Node.ELEMENT_NODE ? root : document;
    if (scope.matches && scope.matches('.panel')) enhanceCard(scope);
    scope.querySelectorAll && scope.querySelectorAll('.panel').forEach(enhanceCard);
    if (scope.matches && scope.matches('button')) enhanceButton(scope);
    scope.querySelectorAll && scope.querySelectorAll('button').forEach(enhanceButton);
    if (scope.matches && scope.matches('input, select, textarea')) enhanceField(scope);
    scope.querySelectorAll && scope.querySelectorAll('input, select, textarea').forEach(enhanceField);
    if (scope.matches && scope.matches('.badge')) enhanceBadge(scope);
    scope.querySelectorAll && scope.querySelectorAll('.badge').forEach(enhanceBadge);
    if (scope.matches && scope.matches('table.data')) enhanceTable(scope);
    scope.querySelectorAll && scope.querySelectorAll('table.data').forEach(enhanceTable);
    scope.querySelectorAll && scope.querySelectorAll('.row-actions, .actions').forEach(function (item) { item.classList.add('btn-list'); });
    scope.querySelectorAll && scope.querySelectorAll('.card-body > .row:first-child').forEach(function (item) { item.classList.add('admin-toolbar'); });
  }

  addPageHeading();
  enhance(document);
  new MutationObserver(function (mutations) {
    const roots = new Set(), tables = new Set();
    mutations.forEach(function (mutation) {
      mutation.addedNodes.forEach(function (node) {
        if (node.nodeType === Node.ELEMENT_NODE) roots.add(node);
      });
      const table = mutation.target.closest && mutation.target.closest('table.data');
      if (table) tables.add(table);
    });
    roots.forEach(function (node) {
      if (!node.isConnected) return;
      for (let parent = node.parentElement; parent; parent = parent.parentElement) {
        if (roots.has(parent)) return;
      }
      enhance(node);
    });
    tables.forEach(function (table) { if (table.isConnected) enhanceTable(table); });
  }).observe(document.body, { childList: true, subtree: true });
})();
