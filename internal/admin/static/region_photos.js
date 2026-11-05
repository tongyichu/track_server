(function () {
  'use strict';

  window.RegionPhotosEditor = class {
    constructor(root, { editable, saved, changed, busyChanged }) {
      this.root = root;
      this.editable = editable;
      this.saved = saved;
      this.changed = changed;
      this.busyChanged = busyChanged;
      this.busy = false;
      this.node = null;
      this.previewURL = '';
      window.addEventListener('pagehide', () => this.dispose());
    }

    bind(node) {
      this.dispose();
      this.node = node;
      this.file = null;
      this.caption = '';
      this.error = '';
      this.dragged = null;
      this.render();
    }

    dispose() {
      this.request?.abort();
      if (this.previewURL) URL.revokeObjectURL(this.previewURL);
      this.previewURL = '';
    }

    button(label, action, className = 'btn-outline-secondary') {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'btn btn-sm ' + className;
      button.textContent = label;
      button.dataset.adminWrite = 'regions';
      button.addEventListener('click', action);
      return button;
    }

    change() {
      this.changed(this.node);
    }

    move(from, to) {
      if (!this.editable() || this.busy || from === to || to < 0 || to >= this.node.photos.length) return;
      const [photo] = this.node.photos.splice(from, 1);
      this.node.photos.splice(to, 0, photo);
      this.change();
      this.render();
    }

    render() {
      this.root.replaceChildren();
      this.root.hidden = !this.node || !['province', 'city'].includes(this.node.level);
      if (this.root.hidden) return;
      const heading = document.createElement('h3');
      heading.textContent = this.node.level === 'province' ? '省份图片' : '城市图片';
      const hint = document.createElement('p');
      hint.className = 'form-hint';
      hint.textContent = '拖动或使用箭头调整顺序，图片说明和顺序随当前节点保存。';
      const grid = document.createElement('div');
      grid.className = 'region-photo-grid';
      this.root.append(heading, hint, grid);
      const photos = this.node.photos || [];
      const writable = this.editable();
      photos.forEach((photo, index) => {
        const card = document.createElement('div');
        card.className = 'region-photo-card';
        card.draggable = writable && !this.busy;
        const image = document.createElement('img');
        image.src = photo.url.startsWith('/api/v1/static/regions/')
          ? photo.url.replace('/api/v1/static/', '/admin/api/static/') : photo.url;
        image.alt = photo.alt_text || photo.caption || photo.title || heading.textContent;
        image.loading = 'lazy';
        image.draggable = false;
        const caption = document.createElement('input');
        caption.className = 'form-control';
        caption.placeholder = '图片说明';
        caption.maxLength = 500;
        caption.value = photo.caption || photo.title || '';
        caption.disabled = !writable;
        caption.setAttribute('aria-label', '第' + (index + 1) + '张图片说明');
        caption.addEventListener('input', () => {
          photo.caption = caption.value;
          delete photo.title;
          this.change();
        });
        card.append(image, caption);
        if (writable) {
          const controls = document.createElement('div');
          controls.className = 'region-photo-actions';
          const left = this.button('←', () => this.move(index, index - 1));
          const right = this.button('→', () => this.move(index, index + 1));
          left.setAttribute('aria-label', '向前移动第' + (index + 1) + '张图片');
          right.setAttribute('aria-label', '向后移动第' + (index + 1) + '张图片');
          left.disabled = index === 0 || this.busy;
          right.disabled = index === photos.length - 1 || this.busy;
          const remove = this.button('移除', () => {
            if (this.busy || !this.editable()) return;
            photos.splice(index, 1);
            this.change();
            this.render();
          }, 'btn-outline-danger');
          remove.disabled = this.busy;
          controls.append(left, right, remove);
          card.appendChild(controls);
        }
        card.addEventListener('dragstart', event => {
          if (!this.editable() || this.busy) { event.preventDefault(); return; }
          this.dragged = index;
          event.dataTransfer.effectAllowed = 'move';
          event.dataTransfer.setData('text/plain', photo.id);
        });
        card.addEventListener('dragover', event => {
          if (Number.isInteger(this.dragged) && !this.busy) event.preventDefault();
        });
        card.addEventListener('drop', event => {
          event.preventDefault();
          if (Number.isInteger(this.dragged)) this.move(this.dragged, index);
          this.dragged = null;
        });
        card.addEventListener('dragend', () => { this.dragged = null; });
        grid.appendChild(card);
      });
      if (!writable) return;
      const card = document.createElement('div');
      card.className = 'region-photo-card';
      const file = document.createElement('input');
      file.type = 'file';
      file.accept = 'image/jpeg,image/png';
      file.hidden = true;
      const picker = this.button('＋ 选择图片', () => file.click());
      picker.className = 'region-photo-picker';
      picker.disabled = this.busy || !this.saved(this.node) || photos.length >= 20;
      if (this.previewURL) {
        const preview = document.createElement('img');
        preview.src = this.previewURL;
        preview.alt = '待上传图片';
        preview.dataset.noPreview = '';
        picker.replaceChildren(preview);
      }
      const caption = document.createElement('input');
      caption.className = 'form-control';
      caption.placeholder = '图片说明';
      caption.maxLength = 500;
      caption.value = this.caption;
      caption.disabled = this.busy || picker.disabled;
      caption.addEventListener('input', () => { this.caption = caption.value; });
      file.addEventListener('change', () => {
        const selected = file.files[0];
        if (!selected) return;
        if (!['image/jpeg', 'image/png'].includes(selected.type) || selected.size === 0 || selected.size > 8 * 1024 * 1024) {
          message.textContent = '请选择8MiB以内的JPG或PNG图片';
          return;
        }
        if (this.previewURL) URL.revokeObjectURL(this.previewURL);
        this.previewURL = URL.createObjectURL(selected);
        this.file = selected;
        this.error = '';
        this.render();
      });
      const upload = this.button(this.busy ? '上传中…' : '上传', () => this.upload(), 'btn-outline-primary');
      upload.disabled = !this.file || picker.disabled;
      const actions = document.createElement('div');
      actions.className = 'region-photo-actions';
      actions.appendChild(upload);
      const message = document.createElement('p');
      message.className = 'form-hint region-photo-message';
      message.setAttribute('role', 'status');
      message.textContent = this.error || (!this.saved(this.node) ? '请先保存当前节点，再上传图片' : photos.length >= 20 ? '最多20张图片' : '支持JPG、PNG，单张不超过8MiB');
      card.append(file, picker, caption, actions);
      grid.appendChild(card);
      this.root.appendChild(message);
    }

    async upload() {
      if (this.busy || !this.file || !this.editable() || !this.saved(this.node)) return;
      const node = this.node;
      const controller = new AbortController();
      this.request = controller;
      this.busy = true;
      this.error = '';
      this.busyChanged();
      this.render();
      try {
        const form = new FormData();
        form.append('file', this.file);
        form.append('caption', this.caption.trim());
        const response = await fetch('/admin/api/regions/' + encodeURIComponent(node.id) + '/photos', {
          method: 'POST', body: form, signal: controller.signal
        });
        const body = await response.json();
        if (!response.ok || body.code !== 0) throw new Error(body.error || '图片上传失败');
        if (this.node !== node || this.request !== controller) return;
        node.photos = [...(node.photos || []), body.data.photo];
        this.file = null;
        this.caption = '';
        this.dispose();
        this.change();
      } catch (error) {
        if (this.node === node && error.name !== 'AbortError') this.error = error.message;
      } finally {
        if (this.request === controller) {
          this.request = null;
          this.busy = false;
          this.busyChanged();
          this.render();
        }
      }
    }
  };
})();
