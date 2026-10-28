(function () {
  const pageMeta = {
    '/admin/index.html': ['App 发布', '安装包上传、版本发布与历史版本管理'],
    '/admin/users.html': ['用户管理', '查看用户资料并管理账号限制'],
    '/admin/tracks.html': ['轨迹管理', '查看、修正或删除用户轨迹'],
    '/admin/scenic_spots.html': ['景点管理', '维护自有景点资料、地图来源与轨迹归属'],
    '/admin/track_submissions.html': ['投稿审核', '审核用户提交的结构化路线资料'],
    '/admin/route_groups.html': ['聚合路线', '维护路线组、代表轨迹和双语介绍'],
    '/admin/companions.html': ['同行管理', '查看同行会话和活动状态'],
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

  function formDialog(options) {
    options = options || {};
    return new Promise(function (resolve) {
      const root = createModal(options);
      const form = document.createElement('form');
      form.className = 'admin-dialog-form';
      (options.fields || []).forEach(function (field) {
        const group = document.createElement('label');
        group.className = 'form-label';
        group.textContent = field.label || field.name;
        const input = document.createElement(field.type === 'textarea' ? 'textarea' : (field.type === 'select' ? 'select' : 'input'));
        input.className = 'form-control';
        input.name = field.name;
        if (field.full) group.classList.add('full');
        if (field.type && field.type !== 'textarea' && field.type !== 'select') input.type = field.type;
        if (field.type === 'select') {
          (field.options || []).forEach(function (option) {
            const item = document.createElement('option');
            item.value = String(option.value);
            item.textContent = String(option.label);
            input.appendChild(item);
          });
        }
        if (field.value != null) input.value = field.value;
        if (field.placeholder) input.placeholder = field.placeholder;
        if (field.required) input.required = true;
        if (field.maxLength) input.maxLength = field.maxLength;
        if (field.min != null) input.min = String(field.min);
        if (field.step != null) input.step = String(field.step);
        group.appendChild(input);
        form.appendChild(group);
      });
      root.querySelector('.modal-body').appendChild(form);
      const footer = root.querySelector('.modal-footer');
      const cancel = document.createElement('button');
      cancel.type = 'button';
      cancel.className = 'btn btn-outline-secondary';
      cancel.textContent = options.cancelText || '取消';
      const submit = document.createElement('button');
      submit.type = 'submit';
      submit.className = 'btn btn-primary';
      submit.textContent = options.confirmText || '保存';
      footer.append(cancel, submit);
      const modal = new tabler.Modal(root, { backdrop: 'static' });
      let settled = false;
      function finish(value) {
        if (settled) return;
        settled = true;
        resolve(value);
        modal.hide();
      }
      cancel.addEventListener('click', function () { finish(null); });
      root.querySelector('.btn-close').addEventListener('click', function () { finish(null); });
      form.addEventListener('submit', function (event) {
        event.preventDefault();
        if (!form.reportValidity()) return;
        const values = {};
        new FormData(form).forEach(function (value, key) { values[key] = String(value); });
        finish(values);
      });
      submit.addEventListener('click', function () { form.requestSubmit(); });
      root.addEventListener('hidden.bs.modal', function () { if (!settled) resolve(null); root.remove(); });
      modal.show();
      setTimeout(function () { const first = form.querySelector('input, textarea'); if (first) first.focus(); }, 150);
    });
  }

  window.AdminUI = { alert: toast, confirm: confirmDialog, form: formDialog };

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
    if (button.hasAttribute('data-period') || button.classList.contains('btn-close')) return;
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
    mutations.forEach(function (mutation) {
      mutation.addedNodes.forEach(function (node) {
        if (node.nodeType === Node.ELEMENT_NODE) enhance(node);
      });
      const table = mutation.target.closest && mutation.target.closest('table.data');
      if (table) enhanceTable(table);
    });
  }).observe(document.body, { childList: true, subtree: true });
})();
