(function () {
  'use strict';

  window.AdminPhotoEditor = class {
    constructor(host, options) {
      this.host = host;
      this.options = options;
      this.items = [];
      this.active = 0;
      this.generation = 0;
      this.waiters = [];
      host.classList.add('admin-photo-grid');
    }

    get busy() { return this.items.some(item => ['queued', 'uploading'].includes(item.state)); }
    get failed() { return this.items.some(item => ['failed', 'invalid'].includes(item.state)); }
    get pending() { return this.items.some(item => item.state !== 'done'); }
    get photos() { return this.items.filter(item => item.state === 'done').map(item => ({ ...item.photo, caption: (item.photo.caption || '').trim() })); }
    get snapshot() { return this.items.map(item => item.state === 'done' ? item.photo : { key: item.key, caption: item.photo.caption, file: item.file?.name, remote: item.remote }); }

    load(photos) {
      this.dispose();
      this.items = (photos || []).map(photo => this.item({ ...photo }, null, 'done'));
      this.render();
    }

    item(photo, file, state) {
      return { key: crypto.randomUUID ? crypto.randomUUID() : String(Date.now()) + Math.random(), photo, file, state, progress: 0, preview: file ? URL.createObjectURL(file) : '', error: '', xhr: null };
    }

    dispose() {
      this.generation++;
      const old = this.items;
      this.items = [];
      this.active = 0;
      old.forEach(item => { item.xhr?.abort(); if (item.preview) URL.revokeObjectURL(item.preview); });
      this.resolveWaiters();
    }

    changed() { this.options.changed?.(); }
    stateChanged() { this.options.stateChanged?.(); this.resolveWaiters(); }
    settle() { return this.busy ? new Promise(resolve => this.waiters.push(resolve)) : Promise.resolve(); }
    resolveWaiters() { if (!this.busy) this.waiters.splice(0).forEach(resolve => resolve()); }

    addPhoto(photo) {
      if (!this.options.editable() || this.items.length >= this.options.max) return;
      this.items.push(this.item({ ...photo }, null, 'done'));
      this.render();
      this.changed();
    }

    addRemotePhotos(candidates, query, page) {
      if (!this.options.editable() || !this.options.importURL) return;
      let added = 0;
      candidates.forEach(candidate => {
        const id = 'pixabay-' + candidate.id;
        if (this.items.length >= this.options.max || this.items.some(item => item.photo.id === id || item.photo.source_url === candidate.source_url)) return;
        const item = this.item({ id, caption: '', source_url: candidate.source_url, provider: 'pixabay' }, null, 'waiting');
        item.remote = { query, page, id: candidate.id };
        item.preview = candidate.preview_url;
        this.items.push(item); added++;
      });
      this.render(); this.start();
      if (added) this.changed();
      return added;
    }

    choose(files) {
      if (!this.options.editable()) return;
      const room = this.options.max - this.items.length;
      const selected = Array.from(files).slice(0, Math.max(0, room));
      selected.forEach(file => {
        const item = this.item({ caption: '' }, file, 'waiting');
        if (!['image/jpeg', 'image/png'].includes(file.type) || !file.size || file.size > 8 * 1024 * 1024) {
          item.state = 'invalid';
          item.error = '请选择8MiB以内的JPG或PNG图片';
          URL.revokeObjectURL(item.preview); item.preview = '';
        }
        this.items.push(item);
      });
      this.render();
      if (files.length > selected.length) this.message.textContent = '最多保留' + this.options.max + '张图片，超出部分未加入';
      this.start();
      this.changed();
    }

    remove(item) {
      if (!this.options.editable()) return;
      this.items = this.items.filter(value => value !== item);
      item.xhr?.abort();
      if (item.preview) URL.revokeObjectURL(item.preview);
      this.render();
      this.changed();
      this.pump();
    }

    move(item, offset) {
      if (!this.options.editable()) return;
      const from = this.items.indexOf(item), to = from + offset;
      if (from < 0 || to < 0 || to >= this.items.length) return;
      this.items.splice(from, 1);
      this.items.splice(to, 0, item);
      this.render();
      this.changed();
    }

    button(text, action, className = 'btn btn-sm btn-outline-secondary') {
      const button = document.createElement('button');
      button.type = 'button'; button.className = className;
      button.dataset.adminWrite = this.options.module;
      button.textContent = text;
      button.addEventListener('click', action);
      return button;
    }

    render() {
      this.host.replaceChildren();
      const writable = this.options.editable();
      this.items.forEach((item, index) => {
        const card = document.createElement('div'); card.className = 'admin-photo-item scenic-photo-item';
        card.dataset.url = item.photo.url || ''; card.dataset.photoId = item.photo.id || item.key;
        const media = document.createElement('div'); media.className = 'admin-photo-media';
        const image = document.createElement('img'); image.draggable = false; image.loading = 'lazy';
        image.alt = item.photo.alt_text || item.photo.caption || item.photo.title || this.options.label;
        const overlay = document.createElement('div'); overlay.className = 'admin-photo-overlay';
        overlay.setAttribute('role', 'status');
        const status = document.createElement('span');
        const progress = document.createElement('progress'); progress.max = 100;
        progress.setAttribute('aria-label', '图片上传进度');
        const retry = this.button(item.remote ? '重新导入' : '重新上传', () => {
          if (!this.options.editable() || item.state !== 'failed') return;
          item.error = ''; item.state = 'waiting'; this.start(); this.update(item);
        }, 'btn btn-sm btn-light');
        overlay.append(status, progress, retry);
        media.append(image, overlay);
        if (writable) {
          const remove = this.button('×', () => this.remove(item), 'admin-photo-remove');
          remove.title = '移除图片'; remove.setAttribute('aria-label', '移除第' + (index + 1) + '张图片');
          media.appendChild(remove);
        }
        const caption = document.createElement('input'); caption.className = 'form-control';
        caption.placeholder = '图片说明'; caption.maxLength = this.options.captionMax || 500;
        caption.value = item.photo.caption ?? item.photo.title ?? '';
        caption.disabled = !writable;
        caption.setAttribute('aria-label', '第' + (index + 1) + '张图片说明');
        caption.addEventListener('input', () => { item.photo.caption = caption.value; delete item.photo.title; this.changed(); });
        card.append(media, caption);
        if (writable) {
          const actions = document.createElement('div'); actions.className = 'admin-photo-order';
          const left = this.button('←', () => this.move(item, -1));
          const right = this.button('→', () => this.move(item, 1));
          left.disabled = index === 0; right.disabled = index === this.items.length - 1;
          left.setAttribute('aria-label', '图片前移'); right.setAttribute('aria-label', '图片后移');
          actions.append(left, right); card.appendChild(actions);
          card.draggable = true;
          card.addEventListener('dragstart', event => { this.dragged = item; event.dataTransfer.setData('text/plain', item.key); });
          card.addEventListener('dragend', () => { this.dragged = null; });
          card.addEventListener('dragover', event => { if (this.dragged) event.preventDefault(); });
          card.addEventListener('drop', event => {
            event.preventDefault(); const dragged = this.dragged; this.dragged = null;
            if (dragged) this.move(dragged, this.items.indexOf(item) - this.items.indexOf(dragged));
          });
        }
        item.view = { card, image, overlay, status, progress, retry };
        this.host.appendChild(card); this.update(item);
      });
      if (writable) {
        const input = document.createElement('input'); input.type = 'file'; input.multiple = true;
        input.accept = 'image/jpeg,image/png'; input.hidden = true;
        const picker = this.button('＋ 选择图片', () => input.click(), 'admin-photo-picker');
        picker.disabled = this.items.length >= this.options.max;
        input.addEventListener('change', () => { const files = Array.from(input.files); input.value = ''; this.choose(files); });
        this.host.append(input, picker);
      }
      this.message = document.createElement('p'); this.message.className = 'form-hint admin-photo-message';
      this.message.textContent = '支持多选JPG、PNG，单张不超过8MiB'; this.host.appendChild(this.message);
    }

    update(item) {
      const ui = item.view;
      if (!ui) return;
      const source = item.preview || this.options.photoURL(item.photo.url || '');
      if (ui.image.getAttribute('src') !== source) { if (source) ui.image.src = source; else ui.image.removeAttribute('src'); }
      ui.card.dataset.url = item.photo.url || '';
      ui.card.dataset.photoId = item.photo.id || item.key;
      ui.overlay.hidden = item.state === 'done';
      ui.progress.hidden = item.state !== 'uploading';
      if (item.remote) ui.progress.removeAttribute('value'); else ui.progress.value = item.progress;
      ui.retry.hidden = item.state !== 'failed' || !this.options.editable();
      ui.status.textContent = item.state === 'waiting' ? (item.remote ? '保存后导入' : '保存后上传') : item.state === 'queued' ? (item.remote ? '等待导入' : '等待上传') : item.state === 'uploading' ? (item.remote ? '正在下载并存储…' : item.progress === 100 ? '处理中…' : item.progress + '%') : item.error;
      ui.image.toggleAttribute('data-no-preview', item.state !== 'done');
    }

    start() {
      if (this.options.uploadURL()) this.items.forEach(item => { if (item.state === 'waiting') item.state = 'queued'; this.update(item); });
      this.pump();
    }

    pump() {
      const url = this.options.uploadURL();
      if (!this.options.editable()) this.items.forEach(item => {
        if (item.state === 'queued') { item.state = 'failed'; item.error = '操作权限已变化'; this.update(item); }
      });
      while (url && this.options.editable() && this.active < 2) {
        const next = this.items.find(item => item.state === 'queued');
        if (!next) break;
        this.upload(next, url);
      }
      this.stateChanged();
    }

    upload(item, url) {
      const generation = this.generation;
      const xhr = new XMLHttpRequest();
      item.xhr = xhr; item.state = 'uploading'; item.progress = 0; this.active++;
      this.update(item);
      let finished = false;
      const finish = error => {
        if (finished) return; finished = true;
        if (generation !== this.generation) return;
        this.active--; item.xhr = null;
        if (this.items.includes(item)) {
          if (error) { item.state = 'failed'; item.error = error; }
          else {
            let body;
            try { body = JSON.parse(xhr.responseText); } catch (_) { body = {}; }
            if (xhr.status < 200 || xhr.status >= 300 || body.code !== 0 || !body.data?.photo?.url) {
              item.state = [400, 410, 413, 415].includes(xhr.status) ? 'invalid' : 'failed';
              item.error = body.error || '上传失败，请重试';
            } else {
              item.photo = { ...body.data.photo, caption: item.photo.caption || '' };
              item.state = 'done'; item.file = null;
              if (item.preview) URL.revokeObjectURL(item.preview); item.preview = '';
              this.changed();
            }
          }
          this.update(item);
        }
        this.pump();
      };
      xhr.open('POST', item.remote ? this.options.importURL() : url); xhr.timeout = 120000;
      xhr.upload.onprogress = event => {
        if (generation !== this.generation || !this.items.includes(item)) return;
        if (!item.remote && event.lengthComputable) item.progress = Math.min(100, Math.floor(event.loaded / event.total * 100));
        this.update(item);
      };
      xhr.onload = () => finish(); xhr.onerror = () => finish('网络异常，请重试');
      xhr.ontimeout = () => finish('上传超时，请重试'); xhr.onabort = () => finish('上传已取消');
      let body;
      if (item.remote) {
        xhr.setRequestHeader('Content-Type', 'application/json'); body = JSON.stringify(item.remote);
      } else {
        body = new FormData(); body.append('file', item.file); body.append('caption', item.photo.caption || '');
      }
      try { xhr.send(body); } catch (_) { finish(item.remote ? '导入失败，请重试' : '上传失败，请重试'); }
    }
  };
})();
