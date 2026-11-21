(function () {
  'use strict';

  class Dialog {
    constructor(root, { backdrop = true, keyboard = true } = {}) {
      this.root = root;
      this.closing = false;
      this.events = new AbortController();
      const options = { signal: this.events.signal };
      let pressedOutside = false;
      root.addEventListener('pointerdown', event => {
        pressedOutside = event.target === root;
      }, options);
      root.addEventListener('click', event => {
        const dismiss = event.target.closest('[data-admin-dismiss="dialog"]');
        if (dismiss?.closest('dialog') === root && !dismiss.disabled) this.hide();
        if (backdrop === true && pressedOutside && event.target === root) this.hide();
        pressedOutside = false;
      }, options);
      root.addEventListener('cancel', event => {
        event.preventDefault();
        if (keyboard) this.hide();
      }, options);
    }

    show() {
      if (this.root.open || this.closing) return;
      menuOpen?.hide();
      this.root.removeAttribute('aria-hidden');
      this.root.showModal();
      this.root.scrollTop = 0;
      this.root.querySelector('.modal-body')?.scrollTo(0, 0);
      this.root.classList.add('show');
      this.root.dispatchEvent(new Event('admin:dialog-shown'));
    }

    hide() {
      if (!this.root.open || this.closing) return;
      this.closing = true;
      const closing = new Event('admin:dialog-closing', { cancelable: true });
      if (!this.root.dispatchEvent(closing)) {
        this.closing = false;
        return;
      }
      this.root.close();
      this.root.classList.remove('show');
      this.root.setAttribute('aria-hidden', 'true');
      this.closing = false;
      this.root.dispatchEvent(new Event('admin:dialog-closed'));
    }

    dispose() {
      this.events.abort();
    }
  }

  function anchorPopup(trigger, popup, { alignEnd = false, preferAbove = false, matchWidth = false,
    minWidth = 0, maxHeight = 360, fillHeight = false, gap = 6, onDetach } = {}) {
    const events = new AbortController();
    const viewport = window.visualViewport;
    const margin = 8;
    popup.classList.add('admin-anchored-popup');
    if (popup.showPopover) {
      popup.setAttribute('popover', 'manual');
      popup.showPopover();
    }

    function place() {
      if (!trigger.isConnected || !popup.isConnected) {
        close();
        onDetach?.();
        return;
      }
      const anchor = trigger.getBoundingClientRect();
      const left = (viewport?.offsetLeft ?? 0) + margin;
      const top = (viewport?.offsetTop ?? 0) + margin;
      const right = left + (viewport?.width ?? document.documentElement.clientWidth) - margin * 2;
      const bottom = top + (viewport?.height ?? innerHeight) - margin * 2;
      const above = Math.max(0, Math.floor(anchor.top - gap - top));
      const below = Math.max(0, Math.floor(bottom - anchor.bottom - gap));
      if (!anchor.height || anchor.bottom <= top || anchor.top >= bottom || !Math.max(above, below)) {
        popup.style.visibility = 'hidden';
        return;
      }
      popup.style.visibility = '';
      popup.style.maxWidth = Math.max(0, right - left) + 'px';
      if (matchWidth) popup.style.minWidth = Math.min(right - left, Math.max(anchor.width, minWidth)) + 'px';
      const contentHeight = popup.scrollHeight + popup.offsetHeight - popup.clientHeight;
      const desired = fillHeight ? maxHeight : Math.min(maxHeight, contentHeight);
      const preferred = preferAbove ? above : below;
      const opposite = preferAbove ? below : above;
      const showAbove = preferred >= desired || preferred >= opposite ? preferAbove : !preferAbove;
      const height = Math.min(maxHeight, showAbove ? above : below);
      popup.style.maxHeight = height + 'px';
      if (fillHeight) popup.style.height = height + 'px';
      const rect = popup.getBoundingClientRect();
      const x = alignEnd ? anchor.right - rect.width : anchor.left;
      popup.style.left = Math.max(left, Math.min(x, right - rect.width)) + 'px';
      popup.style.top = (showAbove ? anchor.top - gap - rect.height : anchor.bottom + gap) + 'px';
    }

    const observer = new ResizeObserver(place);
    observer.observe(trigger);
    observer.observe(popup);
    const options = { signal: events.signal, passive: true };
    window.addEventListener('resize', place, options);
    viewport?.addEventListener('resize', place, options);
    viewport?.addEventListener('scroll', place, options);
    document.addEventListener('scroll', event => {
      if (!popup.contains(event.target)) place();
    }, { ...options, capture: true });
    place();
    function close() {
      if (events.signal.aborted) return;
      events.abort();
      observer.disconnect();
      if (popup.hidePopover && popup.matches(':popover-open')) popup.hidePopover();
      popup.removeAttribute('popover');
      popup.classList.remove('admin-anchored-popup');
      for (const property of ['top', 'left', 'max-height', 'max-width', 'visibility']) popup.style.removeProperty(property);
      if (fillHeight) popup.style.removeProperty('height');
      if (matchWidth) popup.style.removeProperty('min-width');
    }
    return { place, close };
  }

  let menuOpen;
  const dropdowns = new WeakMap();
  const triggerSelector = '[data-admin-menu]';

  class Dropdown {
    static getInstance(trigger) { return dropdowns.get(trigger); }

    constructor(trigger) {
      this.trigger = trigger;
      this.wrapper = trigger.closest('.dropdown');
      this.menu = this.wrapper?.querySelector('.dropdown-menu');
      if (this.menu) dropdowns.set(trigger, this);
    }

    items() {
      return Array.from(this.menu.querySelectorAll('.dropdown-item')).filter(item =>
        !item.disabled && item.getClientRects().length
      );
    }

    show() {
      if (!this.menu || this.trigger.disabled || menuOpen === this) return;
      menuOpen?.hide();
      this.trigger.dispatchEvent(new Event('admin:menu-opening', { bubbles: true }));
      menuOpen = this;
      this.menu.classList.add('show');
      this.trigger.classList.add('show');
      this.trigger.setAttribute('aria-expanded', 'true');
      const anchor = this.trigger.dataset.adminMenuAnchor === 'parent' ? this.wrapper : this.trigger;
      this.anchor = anchorPopup(anchor, this.menu, {
        alignEnd: this.menu.classList.contains('dropdown-menu-end'),
        preferAbove: this.wrapper.classList.contains('dropup'),
        onDetach: () => this.hide()
      });
    }

    hide(restoreFocus = false) {
      if (menuOpen !== this) return;
      menuOpen = null;
      this.anchor?.close();
      this.menu.classList.remove('show');
      this.trigger.classList.remove('show');
      this.trigger.setAttribute('aria-expanded', 'false');
      if (restoreFocus && this.trigger.isConnected) this.trigger.focus({ preventScroll: true });
    }

    dispose() {
      this.hide();
      dropdowns.delete(this.trigger);
    }
  }

  function dropdownFor(trigger) {
    return Dropdown.getInstance(trigger) || new Dropdown(trigger);
  }

  document.addEventListener('click', function (event) {
    const trigger = event.target.closest(triggerSelector);
    if (trigger) {
      const dropdown = dropdownFor(trigger);
      if (menuOpen === dropdown) dropdown.hide();
      else {
        dropdown.show();
        if (menuOpen === dropdown && event.detail === 0) dropdown.items()[0]?.focus();
      }
      return;
    }
    const open = menuOpen;
    if (!open) return;
    if (!open.wrapper.contains(event.target) ||
        (open.trigger.dataset.adminMenuClose !== 'outside' && event.target.closest('.dropdown-item'))) {
      open.hide();
    }
  });

  document.addEventListener('keydown', function (event) {
    const trigger = event.target.closest(triggerSelector);
    const open = menuOpen;
    if (open && event.key === 'Escape' && open.wrapper.contains(event.target)) {
      event.preventDefault();
      event.stopPropagation();
      open.hide(true);
      return;
    }
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
    const dropdown = trigger ? dropdownFor(trigger) : (open?.menu?.contains(event.target) ? open : null);
    if (!dropdown?.menu || dropdown.trigger.disabled) return;
    event.preventDefault();
    dropdown.show();
    const items = dropdown.items();
    if (!items.length) return;
    const index = items.indexOf(document.activeElement);
    const next = event.key === 'ArrowDown' ? index + 1 : (index < 0 ? items.length - 1 : index - 1);
    items[(next + items.length) % items.length].focus();
  }, true);

  document.addEventListener('focusin', event => {
    if (menuOpen && !menuOpen.wrapper.contains(event.target)) menuOpen.hide();
  });
  window.addEventListener('pagehide', () => menuOpen?.hide());

  let openSelect;
  function singleSelect(element) {
    return element instanceof HTMLSelectElement && !element.multiple && element.size <= 1 &&
      element.options.length > 0 && !element.matches(':disabled');
  }

  function highlightOption(state, option) {
    state.active?.classList.remove('is-active');
    state.active = option;
    if (!option) {
      state.select.removeAttribute('aria-activedescendant');
      return;
    }
    option.classList.add('is-active');
    state.select.setAttribute('aria-activedescendant', option.id);
    const row = option.getBoundingClientRect();
    const list = state.popup.getBoundingClientRect();
    if (row.top < list.top) state.popup.scrollTop -= list.top - row.top + 4;
    else if (row.bottom > list.bottom) state.popup.scrollTop += row.bottom - list.bottom + 4;
  }

  function renderOptions(state) {
    const { select, popup } = state;
    const options = document.createDocumentFragment();
    state.items = [];
    function appendOption(option, index, parent) {
      if (option.hidden || option.closest('optgroup')?.hidden) return;
      const item = document.createElement('div');
      item.id = popup.id + '-' + index;
      item.className = 'admin-select-option';
      item.dataset.index = index;
      item.textContent = option.label;
      item.setAttribute('role', 'option');
      item.setAttribute('aria-selected', String(option.selected));
      if (option.matches(':disabled')) item.setAttribute('aria-disabled', 'true');
      else state.items.push(item);
      parent.append(item);
    }
    const groups = new Map();
    Array.from(select.options).forEach((option, index) => {
      const group = option.closest('optgroup');
      if (!group) { appendOption(option, index, options); return; }
      if (group.hidden) return;
      if (!groups.has(group)) {
        const section = document.createElement('div');
        const heading = document.createElement('div');
        section.setAttribute('role', 'group');
        section.setAttribute('aria-label', group.label);
        heading.className = 'admin-select-group';
        heading.textContent = group.label;
        section.append(heading);
        groups.set(group, section);
        options.append(section);
      }
      appendOption(option, index, groups.get(group));
    });
    popup.replaceChildren(options);
    state.position?.place();
    highlightOption(state, state.items.find(item => Number(item.dataset.index) === select.selectedIndex) || state.items[0]);
  }

  function closeSelect() {
    if (!openSelect) return;
    const { select, popup, position, observer } = openSelect;
    openSelect = null;
    observer.disconnect();
    position.close();
    popup.remove();
    select.setAttribute('aria-expanded', 'false');
    select.removeAttribute('aria-controls');
    select.removeAttribute('aria-activedescendant');
  }

  function showSelect(select) {
    closeSelect();
    menuOpen?.hide();
    const popup = document.createElement('div');
    popup.id = 'adminSelectOptions';
    popup.className = 'dropdown-menu show admin-select-popup';
    popup.setAttribute('role', 'listbox');
    const label = select.getAttribute('aria-label') || Array.from(select.labels, node => node.textContent.trim()).join(' ');
    if (label) popup.setAttribute('aria-label', label);
    (select.closest('dialog') || document.body).append(popup);
    select.setAttribute('aria-expanded', 'true');
    select.setAttribute('aria-controls', popup.id);
    const state = { select, popup, active: null, items: [], search: '', searchedAt: 0 };
    renderOptions(state);
    state.position = anchorPopup(select, popup, { matchWidth: true, minWidth: 96, onDetach: closeSelect });
    highlightOption(state, state.active);
    state.observer = new MutationObserver(() => {
      if (!singleSelect(select)) closeSelect();
      else renderOptions(state);
    });
    state.observer.observe(select, {
      subtree: true, childList: true, characterData: true, attributes: true,
      attributeFilter: ['disabled', 'hidden', 'label', 'selected', 'value', 'multiple', 'size']
    });
    openSelect = state;
    popup.addEventListener('mousedown', event => { if (event.target.closest('[role="option"]')) event.preventDefault(); });
    popup.addEventListener('click', event => {
      const option = event.target.closest('[role="option"]');
      if (option) chooseOption(state, option);
    });
  }

  function chooseOption(state, item) {
    if (!item || !state.items.includes(item) || !singleSelect(state.select)) return;
    const { select } = state;
    const next = Number(item.dataset.index);
    closeSelect();
    select.focus({ preventScroll: true });
    if (select.selectedIndex === next) return;
    select.selectedIndex = next;
    select.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
    select.dispatchEvent(new Event('change', { bubbles: true }));
  }

  document.addEventListener('pointerdown', event => {
    if (event.isPrimary && event.button === 0 && singleSelect(event.target)) event.preventDefault();
    if (openSelect && event.target !== openSelect.select && !openSelect.popup.contains(event.target)) closeSelect();
  }, true);
  document.addEventListener('click', event => {
    const select = event.target;
    if (!singleSelect(select)) return;
    event.preventDefault();
    if (openSelect?.select === select) closeSelect();
    else {
      select.focus({ preventScroll: true });
      showSelect(select);
    }
  }, true);
  document.addEventListener('focusin', event => {
    if (openSelect && event.target !== openSelect.select && !openSelect.popup.contains(event.target)) closeSelect();
  });
  document.addEventListener('keydown', event => {
    if (!singleSelect(event.target) || event.isComposing) return;
    const key = event.key;
    const state = openSelect;
    if (state?.select !== event.target) {
      if (['Enter', ' ', 'ArrowDown', 'ArrowUp', 'F4'].includes(key)) {
        event.preventDefault();
        showSelect(event.target);
      }
      return;
    }
    if (key === 'Tab') { closeSelect(); return; }
    if (key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      closeSelect();
    } else if (key === 'Enter' || key === ' ' || key === 'F4') {
      event.preventDefault();
      chooseOption(state, state.active);
    } else if (['ArrowUp', 'ArrowDown', 'Home', 'End', 'PageUp', 'PageDown'].includes(key)) {
      event.preventDefault();
      const index = state.items.indexOf(state.active);
      const direction = key === 'ArrowDown' || key === 'PageDown' ? 1 : -1;
      const step = key.startsWith('Page') ? Math.max(1, Math.floor(state.popup.clientHeight / (state.active?.offsetHeight || 34))) : 1;
      const next = key === 'Home' ? 0 : key === 'End' ? state.items.length - 1 : index + direction * step;
      highlightOption(state, state.items[Math.max(0, Math.min(next, state.items.length - 1))]);
    } else if (key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      const now = Date.now();
      const letter = key.toLocaleLowerCase();
      state.search = now - state.searchedAt < 700 && state.search !== letter ? state.search + letter : letter;
      state.searchedAt = now;
      const index = state.items.indexOf(state.active) + 1;
      const items = state.search.length === 1 ? state.items.slice(index).concat(state.items.slice(0, index)) : state.items;
      const match = items.find(item => item.textContent.trim().toLocaleLowerCase().startsWith(state.search));
      if (match) highlightOption(state, match);
    }
  }, true);
  window.addEventListener('pagehide', closeSelect);

  window.AdminComponents = { Dialog, Dropdown, anchorPopup };
})();
