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
      if (this.menu.showPopover) {
        this.menu.setAttribute('popover', 'manual');
        this.menu.showPopover();
      }
      this.place();
    }

    place() {
      const anchor = (this.trigger.dataset.adminMenuAnchor === 'parent' ? this.wrapper : this.trigger).getBoundingClientRect();
      const { width, height } = this.menu.getBoundingClientRect();
      const margin = 8;
      const offset = 6;
      const right = this.menu.classList.contains('dropdown-menu-end');
      const showAbove = this.wrapper.classList.contains('dropup') ||
        (anchor.bottom + offset + height > innerHeight - margin && anchor.top > height + offset);
      this.menu.style.left = Math.max(margin, Math.min(right ? anchor.right - width : anchor.left, innerWidth - width - margin)) + 'px';
      this.menu.style.top = Math.max(margin, Math.min(showAbove ? anchor.top - height - offset : anchor.bottom + offset, innerHeight - height - margin)) + 'px';
    }

    hide(restoreFocus = false) {
      if (menuOpen !== this) return;
      menuOpen = null;
      if (this.menu.hidePopover && this.menu.matches(':popover-open')) this.menu.hidePopover();
      this.menu.removeAttribute('popover');
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
  window.addEventListener('resize', () => menuOpen?.place());
  document.addEventListener('scroll', event => {
    if (!menuOpen) return;
    if (menuOpen.menu.contains(event.target)) return;
    if (!menuOpen.trigger.isConnected) menuOpen.hide();
    else menuOpen.place();
  }, { capture: true, passive: true });

  window.AdminComponents = { Dialog, Dropdown };
})();
