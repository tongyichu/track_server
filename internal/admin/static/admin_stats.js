(function () {
  'use strict';

  function metric(options, root) {
    options = options || {};
    root = root || document.createElement('article');
    root.classList.add('admin-metric');
    const label = document.createElement('span');
    const value = document.createElement('strong');
    label.className = 'admin-metric-label';
    value.className = 'admin-metric-value';
    label.textContent = options.label || '';
    value.textContent = options.value == null ? '—' : String(options.value);
    if (options.valueId) value.id = options.valueId;
    root.append(label, value);
    if (options.note || options.noteId) {
      const note = document.createElement('small');
      note.className = 'admin-metric-note';
      note.textContent = options.note || '';
      if (options.noteId) note.id = options.noteId;
      root.appendChild(note);
    }
    if (options.tone === 'warning' || options.tone === 'danger') root.classList.add('is-' + options.tone);
    return root;
  }

  function enhance(root) {
    const metrics = Array.from(root.querySelectorAll('[data-admin-metric]'));
    if (root.matches && root.matches('[data-admin-metric]')) metrics.unshift(root);
    metrics.forEach(function (item) {
      if (item.dataset.metricReady === 'true') return;
      item.dataset.metricReady = 'true';
      metric(item.dataset, item);
    });
    root.querySelectorAll('.admin-period-tabs').forEach(function (group) {
      group.setAttribute('role', 'group');
      group.querySelectorAll('button').forEach(function (button) {
        button.type = 'button';
        button.setAttribute('aria-pressed', String(button.classList.contains('active') || button.getAttribute('aria-pressed') === 'true'));
      });
    });
  }

  // Disclosure charts are measured only after expansion; the page owns data and authorization.
  function onExpand(details, render) {
    details.addEventListener('toggle', function () {
      if (details.open) requestAnimationFrame(function () { if (details.open) render(); });
    });
  }

  window.AdminStats = { metric: metric, enhance: enhance, onExpand: onExpand };
  enhance(document);
})();
