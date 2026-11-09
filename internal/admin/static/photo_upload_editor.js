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
      this.savedPhotos = [];
      this.savingPhotos = false;
      this.saveTimer = null;
      this.saveError = '';
      this.infoQueue = [];
      this.infoActive = 0;
      this.infoControllers = new Set();
      this.infoGeneration = 0;
      host.classList.add('admin-photo-grid');
    }

    get ready() { return !this.options.ready || this.options.ready(); }
    get uploading() { return this.items.some(item => ['queued', 'uploading'].includes(item.state)); }
    get busy() { return this.uploading || this.savingPhotos || this.saveTimer !== null; }
    get failed() { return !!this.saveError || this.items.some(item => ['failed', 'invalid'].includes(item.state)); }
    get unsaved() { return !!this.options.savePhotos && (this.fingerprint(this.photosToSave) !== this.fingerprint(this.savedPhotos) || this.items.some(item => (item.photo.caption ?? item.photo.title ?? '').trim() !== item.committedCaption)); }
    get pending() { return this.unsaved || this.items.some(item => item.state !== 'done'); }
    get photos() { return this.items.filter(item => item.state === 'done').map(item => ({ ...item.photo, caption: (item.photo.caption || '').trim() })); }
    get photosToSave() { return this.items.filter(item => item.state === 'done').map(item => ({ ...item.photo, caption: item.committedCaption })); }
    fingerprint(photos) {
      return JSON.stringify((photos || []).map(photo => {
        const value = { ...photo, caption: (photo.caption ?? photo.title ?? '').trim() };
        return Object.fromEntries(Object.keys(value).sort().map(key => [key, value[key]]));
      }));
    }
    get snapshot() { return this.items.map(item => item.state === 'done' ? item.photo : { key: item.key, caption: item.photo.caption, file: item.file?.name, remote: item.remote }); }

    load(photos) {
      this.dispose();
      this.savedPhotos = structuredClone(photos || []);
      this.items = (photos || []).map(photo => this.item({ ...photo }, null, 'done'));
      this.render();
    }

    item(photo, file, state) {
      return { key: crypto.randomUUID ? crypto.randomUUID() : String(Date.now()) + Math.random(), photo, committedCaption: (photo.caption ?? photo.title ?? '').trim(), file, state, info: { size_bytes: file?.size || 0 }, infoState: 'idle', progress: 0, preview: file ? URL.createObjectURL(file) : '', error: '', xhr: null };
    }

    dispose() {
      this.generation++;
      this.pauseInfo();
      clearTimeout(this.saveTimer); this.saveTimer = null;
      this.saveController?.abort(); this.saveController = null;
      this.savedPhotos = []; this.savingPhotos = false; this.saveError = '';
      const old = this.items;
      this.items = [];
      this.active = 0;
      old.forEach(item => { item.xhr?.abort(); item.downloadController?.abort(); if (item.preview) URL.revokeObjectURL(item.preview); });
      this.resolveWaiters();
    }

    changed(commit = true) { this.options.changed?.(); if (commit) this.schedulePhotos(); this.stateChanged(); }
    stateChanged() { this.renderSaveStatus(); this.options.stateChanged?.(); this.resolveWaiters(); }
    settle() { return this.busy ? new Promise(resolve => this.waiters.push(resolve)) : Promise.resolve(); }
    resolveWaiters() { if (!this.busy) this.waiters.splice(0).forEach(resolve => resolve()); }

    schedulePhotos() {
      if (!this.options.savePhotos || !this.ready || !this.options.editable()) return;
      this.saveError = '';
      if (this.savingPhotos || this.saveTimer !== null || this.fingerprint(this.photosToSave) === this.fingerprint(this.savedPhotos)) return;
      this.saveTimer = setTimeout(() => { this.saveTimer = null; this.persistPhotos(); }, 0);
    }

    async persistPhotos() {
      if (!this.options.savePhotos || !this.ready || !this.options.editable() || this.savingPhotos) { this.stateChanged(); return; }
      const photos = structuredClone(this.photosToSave);
      if (this.fingerprint(photos) === this.fingerprint(this.savedPhotos)) { this.stateChanged(); return; }
      const generation = this.generation, controller = new AbortController();
      this.saveController = controller; this.savingPhotos = true; this.saveError = ''; this.stateChanged();
      const timeout = setTimeout(() => controller.abort(), 30000);
      try {
        const result = await this.options.savePhotos(photos, structuredClone(this.savedPhotos), controller.signal);
        if (generation !== this.generation) return;
        const savedPhotos = this.options.photosFromResult?.(result) || photos;
        this.savedPhotos = structuredClone(savedPhotos);
        // A server import may replace remote URLs while caption edits remain in progress.
        this.items.forEach(item => {
          if (item.state !== 'done') return;
          const sent = photos.find(photo => photo.id === item.photo.id && photo.url === item.photo.url);
          const saved = sent && savedPhotos.find(photo => photo.id === sent.id);
          if (!saved) return;
          const previousURL = item.photo.url;
          item.photo = { ...saved, caption: item.photo.caption };
          if (previousURL !== item.photo.url) {
            item.info = {}; item.infoState = 'idle';
            if (item.view) {
              item.view.card.dataset.url = item.photo.url;
              item.view.image.src = this.options.photoURL(item.photo.url);
            }
            this.queueInfo(item); this.renderInfo(item);
          }
        });
        this.options.photosSaved?.(result, photos);
      } catch (error) {
        if (generation === this.generation) this.saveError = error.name === 'AbortError' ? '图片保存超时，请重试' : error.message;
      } finally {
        clearTimeout(timeout);
        if (generation === this.generation) {
          this.savingPhotos = false; this.saveController = null;
          if (!this.saveError) this.schedulePhotos();
          this.stateChanged();
        }
      }
    }

    renderSaveStatus() {
      if (!this.saveStatus) return;
      const uploadsFailed = this.items.some(item => ['failed', 'invalid'].includes(item.state));
      this.saveStatus.text.textContent = !this.ready ? '请先保存基本信息，再添加图片' : this.saveError ? '图片保存失败：' + this.saveError : this.savingPhotos || this.saveTimer !== null ? '图片保存中…' : this.uploading ? '图片上传中，完成后自动保存' : uploadsFailed ? '已上传图片已保存，失败项请重试或移除' : this.unsaved ? '图片说明将在离开输入框后自动保存' : '图片已自动保存';
      this.saveStatus.text.className = this.saveError ? 'text-danger' : 'text-secondary';
      this.saveStatus.retry.hidden = !this.saveError;
      this.saveStatus.discard.hidden = !this.saveError || this.busy;
    }

    addPhoto(photo) {
      if (!this.options.editable() || this.items.length >= this.options.max) return;
      this.items.push(this.item({ ...photo }, null, 'done'));
      this.render();
      this.changed();
    }

    addRemotePhotos(candidates, query, page) {
      if (!this.options.editable() || !this.ready || !this.options.importURL) return;
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
      if (!this.options.editable() || !this.ready) return;
      const room = this.options.max - this.items.length;
      const selected = Array.from(files).slice(0, Math.max(0, room));
      selected.forEach(file => {
        const item = this.item({ caption: '' }, file, 'waiting');
        const supported = ['image/jpeg', 'image/png', 'image/webp'].includes(file.type) || !file.type && /\.(jpe?g|png|webp)$/i.test(file.name);
        if (!supported || !file.size || file.size > 8 * 1024 * 1024) {
          item.state = 'invalid';
          item.error = '请选择8MiB以内的JPG、PNG或WebP图片';
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
      item.downloadController?.abort();
      item.infoController?.abort();
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

    async downloadPhoto(item) {
      if (item.state !== 'done' || !item.photo.url || item.downloadController || !this.items.includes(item)) return;
      const generation = this.generation, controller = new AbortController();
      item.downloadController = controller;
      this.updateDownload(item);
      let timedOut = false;
      const timeout = setTimeout(() => { timedOut = true; controller.abort(); }, 30000);
      try {
        const source = new URL(this.options.photoURL(item.photo.url), window.location.href);
        if (!['http:', 'https:'].includes(source.protocol)) throw new Error('图片地址无效');
        const response = await fetch(source.href, { signal: controller.signal, credentials: 'same-origin' });
        if (!response.ok || !/^image\//i.test(response.headers.get('Content-Type') || '')) throw new Error('图片下载失败，请重试');
        const image = await response.blob();
        if (!image.size) throw new Error('图片内容为空');
        if (generation !== this.generation || !this.items.includes(item) || controller.signal.aborted) return;
        const link = document.createElement('a');
        let name = source.pathname.split('/').pop() || 'image';
        try { name = decodeURIComponent(name); } catch (_) { /* Keep the encoded filename if decoding fails. */ }
        link.download = name.replace(/[\\/:*?"<>|\u0000-\u001f]/g, '_');
        link.href = URL.createObjectURL(image);
        setTimeout(() => URL.revokeObjectURL(link.href), 60000);
        link.hidden = true;
        document.body.appendChild(link);
        link.click(); link.remove();
      } catch (error) {
        if (generation === this.generation && this.items.includes(item) && item.view?.card.isConnected &&
            (!controller.signal.aborted || timedOut)) {
          this.message.textContent = error.name === 'AbortError' ? '图片下载超时，请重试' : '下载失败：' + error.message;
        }
      } finally {
        clearTimeout(timeout);
        if (item.downloadController === controller) {
          item.downloadController = null;
          if (generation === this.generation && this.items.includes(item)) this.updateDownload(item);
        }
      }
    }

    render() {
      this.items.forEach(item => { if (item.view?.caption === document.activeElement) item.committedCaption = (item.photo.caption || '').trim(); });
      this.host.replaceChildren();
      const writable = this.options.editable() && this.ready;
      this.items.forEach((item, index) => {
        const card = document.createElement('div'); card.className = 'admin-photo-item scenic-photo-item';
        card.dataset.url = item.photo.url || ''; card.dataset.photoId = item.photo.id || item.key;
        const media = document.createElement('div'); media.className = 'admin-photo-media';
        const image = document.createElement('img'); image.draggable = false; image.loading = 'lazy';
        image.alt = item.photo.alt_text || item.photo.caption || item.photo.title || this.options.label;
        image.addEventListener('load', () => {
          if (!this.items.includes(item) || item.view?.image !== image || item.remote && item.state !== 'done') return;
          const source = item.preview || this.options.photoURL(item.photo.url || '');
          try { if (!source || image.currentSrc !== new URL(source, window.location.href).href) return; }
          catch (_) { return; }
          if (image.naturalWidth && image.naturalHeight) {
            item.info.width = image.naturalWidth; item.info.height = image.naturalHeight;
            this.renderInfo(item);
          }
        });
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
        const download = document.createElement('button');
        download.type = 'button'; download.className = 'admin-photo-download'; download.hidden = true;
        download.title = '下载原图'; download.setAttribute('aria-label', '下载第' + (index + 1) + '张图片');
        download.innerHTML = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 17v3h16v-3M12 3v12m-4-4 4 4 4-4"/></svg>';
        download.addEventListener('click', event => { event.stopPropagation(); this.downloadPhoto(item); });
        media.appendChild(download);
        if (writable) {
          const remove = this.button('×', () => this.remove(item), 'admin-photo-remove');
          remove.title = '移除图片'; remove.setAttribute('aria-label', '移除第' + (index + 1) + '张图片');
          card.appendChild(remove);
        }
        const caption = document.createElement('input'); caption.className = 'form-control';
        caption.placeholder = '图片说明'; caption.maxLength = this.options.captionMax || 500;
        caption.value = item.photo.caption ?? item.photo.title ?? '';
        caption.disabled = !writable;
        caption.setAttribute('aria-label', '第' + (index + 1) + '张图片说明');
        caption.addEventListener('input', () => { item.photo.caption = caption.value; delete item.photo.title; this.changed(false); });
        caption.addEventListener('blur', () => { item.committedCaption = caption.value.trim(); this.changed(); });
        const info = document.createElement('div'); info.className = 'admin-photo-info';
        card.append(media, info, caption);
        if (writable) {
          card.draggable = true;
          card.addEventListener('dragstart', event => {
            if (event.target instanceof Element && event.target.closest('button, a, input')) { event.preventDefault(); return; }
            this.dragged = item; event.dataTransfer.setData('text/plain', item.key);
          });
          card.addEventListener('dragend', () => { this.dragged = null; });
          card.addEventListener('dragover', event => { if (this.dragged) event.preventDefault(); });
          card.addEventListener('drop', event => {
            event.preventDefault(); const dragged = this.dragged; this.dragged = null;
            if (dragged) this.move(dragged, this.items.indexOf(item) - this.items.indexOf(dragged));
          });
        }
        item.view = { card, image, overlay, status, progress, retry, caption, info, download };
        this.host.appendChild(card); this.update(item);
      });
      if (writable) {
        const input = document.createElement('input'); input.type = 'file'; input.multiple = true;
        input.accept = 'image/jpeg,image/png,image/webp,.jpg,.jpeg,.png,.webp'; input.hidden = true;
        const picker = this.button('＋ 选择图片', () => input.click(), 'admin-photo-picker');
        picker.disabled = this.items.length >= this.options.max;
        input.addEventListener('change', () => { const files = Array.from(input.files); input.value = ''; this.choose(files); });
        this.host.append(input, picker);
      }
      this.message = document.createElement('p'); this.message.className = 'form-hint admin-photo-message';
      this.message.textContent = '支持多选JPG、PNG、WebP，单张不超过8MiB'; this.host.appendChild(this.message);
      this.saveStatus = null;
      if (this.options.savePhotos) {
        const row = document.createElement('div'); row.className = 'admin-photo-save-status'; row.setAttribute('role', 'status');
        const text = document.createElement('span');
        const retry = this.button('重试保存', () => { this.schedulePhotos(); this.stateChanged(); });
        const discard = this.button('放弃本地图片修改', () => { if (!this.busy) { const photos = structuredClone(this.savedPhotos); this.load(photos); this.options.changed?.(); this.stateChanged(); } });
        row.append(text, retry, discard); this.host.appendChild(row);
        this.saveStatus = { text, retry, discard }; this.renderSaveStatus();
      }
    }

    updateDownload(item) {
      const button = item.view?.download;
      if (!button) return;
      button.hidden = item.state !== 'done' || !item.photo.url;
      button.disabled = !!item.downloadController;
      button.title = item.downloadController ? '正在下载…' : '下载原图';
    }

    update(item) {
      const ui = item.view;
      if (!ui) return;
      const source = item.preview || this.options.photoURL(item.photo.url || '');
      if (ui.image.getAttribute('src') !== source) { if (source) ui.image.src = source; else ui.image.removeAttribute('src'); }
      ui.card.dataset.url = item.photo.url || '';
      ui.card.dataset.photoId = item.photo.id || item.key;
      ui.overlay.hidden = item.state === 'done';
      this.updateDownload(item);
      ui.progress.hidden = item.state !== 'uploading';
      if (item.remote) ui.progress.removeAttribute('value'); else ui.progress.value = item.progress;
      ui.retry.hidden = item.state !== 'failed' || !this.options.editable();
      ui.status.textContent = item.state === 'waiting' ? (item.remote ? '保存后导入' : '保存后上传') : item.state === 'queued' ? (item.remote ? '等待导入' : '等待上传') : item.state === 'uploading' ? (item.remote ? '正在下载并存储…' : item.progress === 100 ? '处理中…' : item.progress + '%') : item.error;
      ui.image.toggleAttribute('data-no-preview', item.state !== 'done');
      this.queueInfo(item);
      this.renderInfo(item);
    }

    renderInfo(item) {
      if (!item.view) return;
      if (item.remote && item.state !== 'done') { item.view.info.textContent = '导入后显示尺寸和大小'; return; }
      const info = item.info;
      const loading = ['queued', 'loading'].includes(item.infoState) || !!item.preview && item.state !== 'invalid';
      const dimensions = info.width > 0 && info.height > 0 ? info.width + ' × ' + info.height + ' px' : loading ? '尺寸读取中' : '尺寸未知';
      const bytes = info.size_bytes;
      let size = loading ? '大小读取中' : '文件大小未知';
      if (Number.isSafeInteger(bytes) && bytes > 0) {
        size = bytes < 1024 ? bytes + ' B' : bytes < 1024 * 1024 ? Number((bytes / 1024).toFixed(1)) + ' KB' : Number((bytes / (1024 * 1024)).toFixed(1)) + ' MB';
      }
      item.view.info.textContent = dimensions + ' · ' + size;
    }

    queueInfo(item) {
      if (item.state !== 'done' || item.infoState !== 'idle') return;
      if (item.info.width > 0 && item.info.height > 0 && item.info.size_bytes > 0) { item.infoState = 'done'; return; }
      let url;
      try { url = new URL(this.options.photoURL(item.photo.url || ''), window.location.href); }
      catch (_) { item.infoState = 'done'; return; }
      if (url.origin !== window.location.origin || !/^\/admin\/api\/static\/(regions|scenic_spots)\//.test(url.pathname)) { item.infoState = 'done'; return; }
      url.searchParams.set('photo_info', '1');
      item.infoState = 'queued';
      this.infoQueue.push({ item, url: url.href, generation: this.infoGeneration });
      this.pumpInfo();
    }

    pauseInfo() {
      this.infoGeneration++;
      this.infoControllers.forEach(controller => controller.abort()); this.infoControllers.clear();
      this.infoQueue = []; this.infoActive = 0;
      this.items.forEach(item => {
        item.downloadController?.abort();
        if (['queued', 'loading'].includes(item.infoState)) item.infoState = 'idle';
        item.infoController = null;
      });
    }

    pumpInfo() {
      while (this.infoActive < 3 && this.infoQueue.length) {
        const job = this.infoQueue.shift();
        if (job.generation !== this.infoGeneration || !this.items.includes(job.item)) continue;
        const controller = new AbortController();
        job.item.infoController = controller; job.item.infoState = 'loading';
        this.infoControllers.add(controller); this.infoActive++;
        this.readInfo(job, controller);
      }
    }

    async readInfo(job, controller) {
      const timeout = setTimeout(() => controller.abort(), 10000);
      try {
        const response = await fetch(job.url, { signal: controller.signal });
        const body = await response.json();
        if (job.generation !== this.infoGeneration || !this.items.includes(job.item)) return;
        if (response.ok && body.code === 0 && body.data) {
          ['width', 'height', 'size_bytes'].forEach(key => {
            const value = body.data[key];
            if (Number.isSafeInteger(value) && value > 0) job.item.info[key] = value;
          });
        }
      } catch (_) { /* Image facts never block uploads or edits. */ }
      finally {
        clearTimeout(timeout);
        if (job.generation === this.infoGeneration) {
          this.infoControllers.delete(controller); this.infoActive--;
          job.item.infoController = null; job.item.infoState = 'done';
          if (this.items.includes(job.item)) this.renderInfo(job.item);
          this.pumpInfo();
        }
      }
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
              item.info = body.data.image_info || { size_bytes: body.data.size || item.file?.size || 0 };
              item.infoState = 'idle';
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
