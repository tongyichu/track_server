(function () {
  'use strict';
  const instances = new WeakMap();
  let sequence = 0;

  function enhance(province, city, regions, options) {
    options = options || {};
    if (instances.has(province)) { const existing = instances.get(province); existing.setRegions(regions); return existing; }
    const provinceSource = province.closest('label') || province;
    const citySource = city.closest('label') || city;
    const host = document.createElement('div'); host.className = 'admin-region-field';
    const label = document.createElement('span'); label.className = 'admin-region-label'; label.textContent = options.label || '地区';
    const toggle = document.createElement('button'); toggle.type = 'button'; toggle.className = 'form-select admin-region-toggle';
    const popup = document.createElement('div'); popup.className = 'admin-region-popup'; popup.hidden = true;
    popup.id = 'adminRegionPopup' + (++sequence); popup.setAttribute('role', 'dialog'); popup.setAttribute('aria-label', label.textContent);
    toggle.setAttribute('aria-haspopup', 'dialog'); toggle.setAttribute('aria-controls', popup.id); toggle.setAttribute('aria-expanded', 'false');
    const back = document.createElement('button'); back.type = 'button'; back.className = 'btn btn-sm btn-outline-secondary'; back.textContent = '返回省份';
    const heading = document.createElement('div'); heading.className = 'admin-region-heading';
    const title = document.createElement('strong'); heading.append(back, title);
    const list = document.createElement('div'); list.className = 'admin-region-options';
    popup.append(heading, list); host.append(label, toggle); provinceSource.before(host); document.body.appendChild(popup);
    provinceSource.classList.add('admin-region-source'); citySource.classList.add('admin-region-source');
    [...province.labels, ...city.labels].forEach(source => source.classList.add('admin-region-source'));
    let data = regions || [], browsingProvince = '', stage = 'province';
    const allowAll = options.allowAll !== false;

    function sync() {
      const p = data.find(item => item.code === province.value), c = data.find(item => item.code === city.value);
      toggle.textContent = c ? (p && p.code !== c.code ? p.name + ' / ' : '') + c.name : p ? p.name + ' / 全省' : allowAll ? '全部地区' : '选择省份 / 城市';
      toggle.setAttribute('aria-label', label.textContent + '：' + toggle.textContent);
      toggle.disabled = province.disabled || !data.length;
      if (toggle.disabled) close(false);
    }
    function close(focus) { popup.hidden = true; toggle.setAttribute('aria-expanded', 'false'); if (focus) toggle.focus(); }
    function position() {
      const rect = toggle.getBoundingClientRect(), width = Math.min(360, window.innerWidth - 24);
      popup.style.width = width + 'px'; popup.style.left = Math.max(12, Math.min(rect.left, window.innerWidth - width - 12)) + 'px';
      const below = window.innerHeight - rect.bottom - 12, above = rect.top - 12;
      const useBelow = below >= Math.min(320, above);
      const height = Math.max(90, Math.min(360, useBelow ? below : above));
      popup.style.maxHeight = height + 'px';
      popup.style.top = (useBelow ? rect.bottom + 4 : Math.max(12, rect.top - Math.min(popup.scrollHeight, height) - 4)) + 'px';
    }
    function commit(provinceCode, cityCode) {
      if (province.disabled) return;
      const changed = province.value !== provinceCode || city.value !== cityCode;
      province.value = provinceCode;
      const cities = data.filter(item => item.level === 'city' && (!provinceCode || item.province_code === provinceCode));
      city.replaceChildren(new Option(provinceCode ? '全省' : '全部', ''));
      cities.forEach(item => city.add(new Option(item.name, item.code)));
      city.disabled = false; city.value = cityCode;
      sync(); close(true);
      if (changed) city.dispatchEvent(new Event('change', { bubbles: true }));
    }
    function option(text, selected, choose) {
      const button = document.createElement('button'); button.type = 'button'; button.className = 'admin-region-option';
      button.textContent = text; button.setAttribute('aria-pressed', String(selected)); button.addEventListener('click', choose); list.appendChild(button);
    }
    function render(focus) {
      list.replaceChildren(); back.hidden = stage === 'province';
      const current = data.find(item => item.code === browsingProvince);
      title.textContent = stage === 'province' ? '选择省份' : current ? current.name + ' · 选择城市' : '全部省份 · 选择城市';
      if (stage === 'province') {
        function openCities(code) { browsingProvince = code; stage = 'city'; render(true); }
        if (allowAll) option('全部省份', !province.value, () => openCities(''));
        data.filter(item => item.level === 'province').forEach(item => option(item.name, province.value === item.code, () => openCities(item.code)));
      } else {
        option(browsingProvince ? '全省' : '全部', province.value === browsingProvince && !city.value, () => commit(browsingProvince, ''));
        const cities = data.filter(item => item.level === 'city' && (!browsingProvince || item.province_code === browsingProvince));
        cities.forEach(item => {
          const parent = !browsingProvince && data.find(region => region.code === item.province_code);
          option((parent && parent.name !== item.name ? parent.name + ' / ' : '') + item.name, city.value === item.code, () => commit(browsingProvince, item.code));
        });
      }
      position();
      if (focus) (list.querySelector('[aria-pressed="true"]') || list.querySelector('button'))?.focus();
    }
    function open() { sync(); if (toggle.disabled) return; stage = 'province'; popup.hidden = false; toggle.setAttribute('aria-expanded', 'true'); render(true); }
    toggle.addEventListener('click', () => popup.hidden ? open() : close(false));
    toggle.addEventListener('keydown', event => { if (event.key === 'ArrowDown') { event.preventDefault(); open(); } });
    back.addEventListener('click', () => { stage = 'province'; render(true); });
    popup.addEventListener('keydown', event => {
      const buttons = Array.from(list.querySelectorAll('button')), index = buttons.indexOf(document.activeElement);
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true); }
      else if (event.key === 'ArrowLeft' && stage === 'city') { event.preventDefault(); stage = 'province'; render(true); }
      else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
        event.preventDefault(); const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : Math.max(0, Math.min(buttons.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1))); buttons[next]?.focus();
      } else if (event.key === 'Tab') close(false);
    });
    document.addEventListener('pointerdown', event => { if (!host.contains(event.target) && !popup.contains(event.target)) close(false); });
    document.addEventListener('focusin', event => { if (!host.contains(event.target) && !popup.contains(event.target)) close(false); });
    document.addEventListener('scroll', event => { if (!popup.contains(event.target)) close(false); }, true);
    window.addEventListener('resize', () => close(false)); window.addEventListener('pagehide', () => close(false));
    province.addEventListener('change', sync); city.addEventListener('change', sync);
    const observer = new MutationObserver(sync);
    observer.observe(province, { childList: true, subtree: true, attributes: true, attributeFilter: ['disabled'] });
    observer.observe(city, { childList: true, subtree: true });
    const api = { setRegions(items) { data = items || []; sync(); }, sync, close: () => close(false) };
    instances.set(province, api); sync(); return api;
  }
  window.AdminRegionPicker = { enhance };
})();
