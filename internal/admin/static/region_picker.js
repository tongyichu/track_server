(function () {
  'use strict';
  const instances = new WeakMap();
  let sequence = 0;

  function enhance(province, city, regions, options) {
    options = options || {};
    if (instances.has(province)) { const existing = instances.get(province); existing.setRegions(regions); return existing; }
    const allowAll = options.allowAll !== false;
    const provinceSource = province.closest('label') || province;
    const citySource = city.closest('label') || city;
    const host = document.createElement('div'); host.className = 'admin-region-field';
    const label = document.createElement('span'); label.className = 'admin-region-label'; label.textContent = options.label || '地区';
    const toggle = document.createElement('button'); toggle.type = 'button'; toggle.className = 'form-select admin-region-toggle';
    const control = document.createElement('div'); control.className = 'admin-region-control';
    const clear = document.createElement('button'); clear.type = 'button'; clear.className = 'admin-region-clear'; clear.textContent = '×'; clear.hidden = true;
    clear.title = '清除地区筛选'; clear.setAttribute('aria-label', clear.title);
    const popup = document.createElement('div'); popup.className = 'admin-region-popup'; popup.hidden = true;
    popup.id = 'adminRegionPopup' + (++sequence); popup.setAttribute('role', 'dialog'); popup.setAttribute('aria-label', label.textContent);
    toggle.setAttribute('aria-haspopup', 'dialog'); toggle.setAttribute('aria-controls', popup.id); toggle.setAttribute('aria-expanded', 'false');
    function column(titleText) {
      const pane = document.createElement('div'); pane.className = 'admin-region-column';
      const heading = document.createElement('div'); heading.className = 'admin-region-column-title'; heading.textContent = titleText;
      const list = document.createElement('div'); list.className = 'admin-region-options';
      list.setAttribute('role', 'group'); list.setAttribute('aria-label', titleText);
      pane.append(heading, list); popup.appendChild(pane); return { pane, heading, list };
    }
    const provinces = column('省份'), cities = column('城市');
    cities.list.id = popup.id + 'Cities';
    control.append(toggle, clear); host.append(label, control); provinceSource.before(host); document.body.appendChild(popup);
    provinceSource.classList.add('admin-region-source'); citySource.classList.add('admin-region-source');
    [...province.labels, ...city.labels].forEach(source => source.classList.add('admin-region-source'));
    let data = regions || [], browsingProvince = '', renderedProvince = null;

    function childCities(code) { return data.filter(item => item.level === 'city' && item.province_code === code); }

    function sync() {
      const p = data.find(item => item.code === province.value), c = data.find(item => item.code === city.value);
      toggle.textContent = c ? (p && p.code !== c.code ? p.name + ' / ' : '') + c.name : p ? p.name + (childCities(p.code).length ? ' / 全部' : '') : allowAll ? '全部地区' : '选择省份 / 城市';
      toggle.setAttribute('aria-label', label.textContent + '：' + toggle.textContent);
      toggle.disabled = province.disabled || !data.length;
      clear.hidden = !allowAll || toggle.disabled || !(province.value || city.value);
      clear.disabled = toggle.disabled; toggle.classList.toggle('is-clearable', !clear.hidden);
      if (toggle.disabled) close(false);
    }
    function close(focus) { popup.hidden = true; toggle.setAttribute('aria-expanded', 'false'); if (focus) toggle.focus(); }
    function position() {
      const rect = toggle.getBoundingClientRect(), fullWidth = Math.min(400, window.innerWidth - 24), provinceWidth = Math.round(fullWidth * .4);
      popup.style.setProperty('--admin-region-province-width', provinceWidth + 'px');
      popup.style.width = (cities.pane.hidden ? provinceWidth : fullWidth) + 'px';
      popup.style.left = Math.max(12, Math.min(rect.left, window.innerWidth - fullWidth - 12)) + 'px';
      const below = window.innerHeight - rect.bottom - 12, above = rect.top - 12;
      const useBelow = below >= Math.min(320, above);
      const height = Math.max(90, Math.min(360, (useBelow ? below : above) - 4));
      popup.style.height = height + 'px';
      popup.style.top = (useBelow ? rect.bottom + 4 : Math.max(12, rect.top - height - 4)) + 'px';
    }
    function commit(provinceCode, cityCode) {
      if (province.disabled) return;
      const changed = province.value !== provinceCode || city.value !== cityCode;
      province.value = provinceCode;
      const cities = data.filter(item => item.level === 'city' && (!provinceCode || item.province_code === provinceCode));
      city.replaceChildren(new Option('全部', ''));
      cities.forEach(item => city.add(new Option(item.name, item.code)));
      city.disabled = false; city.value = cityCode;
      sync(); close(true);
      if (changed) city.dispatchEvent(new Event('change', { bubbles: true }));
    }
    function option(list, text, selected, choose) {
      const button = document.createElement('button'); button.type = 'button'; button.className = 'admin-region-option';
      button.textContent = text; button.title = text; button.setAttribute('aria-pressed', String(selected));
      button.addEventListener('click', choose); list.appendChild(button); return button;
    }
    function focusOption(list) { (list.querySelector('[aria-pressed="true"]') || list.querySelector('button'))?.focus(); }
    function browse(code) {
      browsingProvince = code;
      const children = childCities(code), hasChildren = children.length > 0;
      provinces.list.querySelectorAll('button').forEach(button => {
        const active = button.dataset.provinceCode === code;
        button.classList.toggle('is-browsing', active);
        if (button.hasAttribute('aria-controls')) button.setAttribute('aria-expanded', String(active && hasChildren));
      });
      if (renderedProvince === code) return;
      renderedProvince = code;
      cities.pane.hidden = !hasChildren; popup.classList.toggle('has-cities', hasChildren);
      const current = data.find(item => item.code === code);
      cities.heading.textContent = current ? current.name : '城市';
      cities.list.setAttribute('aria-label', cities.heading.textContent + '城市');
      cities.list.replaceChildren(); cities.list.scrollTop = 0;
      if (hasChildren) {
        option(cities.list, '全部', province.value === code && !city.value, () => commit(code, ''));
        children.forEach(item => option(cities.list, item.name, city.value === item.code, () => commit(code, item.code)));
      }
      if (!popup.hidden) position();
    }
    function render() {
      provinces.list.replaceChildren(); renderedProvince = null;
      function provinceOption(code, name) {
        const hasChildren = childCities(code).length > 0;
        const button = option(provinces.list, name, province.value === code, () => hasChildren ? browse(code) : commit(code, ''));
        button.classList.add('admin-region-province'); button.dataset.provinceCode = code;
        if (hasChildren) {
          button.classList.add('has-children');
          button.setAttribute('aria-controls', cities.list.id); button.setAttribute('aria-expanded', 'false');
        }
        button.addEventListener('pointerenter', event => { if (event.pointerType !== 'touch') browse(code); });
        button.addEventListener('focus', () => browse(code));
      }
      data.filter(item => item.level === 'province').forEach(item => provinceOption(item.code, item.name));
      const initial = province.value || data.find(item => item.level === 'province')?.code || '';
      browse(initial);
    }
    function open(focus) {
      sync(); if (toggle.disabled) return;
      popup.hidden = false; toggle.setAttribute('aria-expanded', 'true'); render(); position();
      if (focus) focusOption(provinces.list);
      else provinces.list.querySelector('.is-browsing')?.scrollIntoView({ block: 'nearest' });
    }
    toggle.addEventListener('click', event => popup.hidden ? open(event.detail === 0) : close(false));
    clear.addEventListener('click', () => commit('', ''));
    toggle.addEventListener('keydown', event => { if (event.key === 'ArrowDown') { event.preventDefault(); open(true); } });
    popup.addEventListener('keydown', event => {
      const list = provinces.list.contains(document.activeElement) ? provinces.list : cities.list;
      const buttons = Array.from(list.querySelectorAll('button')), index = buttons.indexOf(document.activeElement);
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true); }
      else if (event.key === 'ArrowRight' && list === provinces.list) {
        event.preventDefault(); browse(document.activeElement.dataset.provinceCode); if (!cities.pane.hidden) focusOption(cities.list);
      } else if (event.key === 'ArrowLeft' && list === cities.list) {
        event.preventDefault(); Array.from(provinces.list.querySelectorAll('button')).find(button => button.dataset.provinceCode === browsingProvince)?.focus();
      } else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
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
    const api = { setRegions(items) { close(false); data = items || []; sync(); }, sync, close: () => close(false) };
    instances.set(province, api); sync(); return api;
  }
  window.AdminRegionPicker = { enhance };
})();
