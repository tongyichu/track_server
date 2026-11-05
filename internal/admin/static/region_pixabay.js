(function () {
  'use strict';
  window.RegionPixabayPicker = class {
    constructor(host, options) {
      this.host = host; this.options = options;
      this.open = false; this.loading = false; this.generation = 0;
      this.result = null; this.selected = new Set(); this.message = '';
      this.query = options.node().name_en || options.node().name || '';
      this.button = this.action('从 Pixabay 查询', () => {
        this.open = !this.open;
        if (!this.open) this.pause();
        this.panel.hidden = !this.open;
        this.button.setAttribute('aria-expanded', String(this.open));
        if (this.open && !this.result) this.search(1);
      });
      this.button.setAttribute('aria-expanded', 'false');
      this.panel = document.createElement('div'); this.panel.className = 'region-pixabay'; this.panel.hidden = true;
      const toolbar = document.createElement('div'); toolbar.className = 'region-pixabay-toolbar';
      this.input = document.createElement('input'); this.input.className = 'form-control'; this.input.maxLength = 100;
      this.input.value = this.query; this.input.placeholder = '输入地区或地标名称'; this.input.setAttribute('aria-label', 'Pixabay 查询关键词');
      this.input.addEventListener('keydown', event => { if (event.key === 'Enter') { event.preventDefault(); this.search(1); } });
      this.searchButton = this.action('查询', () => this.search(1));
      this.nextButton = this.action('换一组', () => this.search((this.result?.page || 0) + 1));
      const source = document.createElement('a'); source.href = 'https://pixabay.com/'; source.target = '_blank'; source.rel = 'noopener noreferrer'; source.textContent = '图片来自 Pixabay';
      toolbar.append(this.input, this.searchButton, this.nextButton, source);
      this.status = document.createElement('p'); this.status.className = 'form-hint'; this.status.setAttribute('role', 'status');
      this.grid = document.createElement('div'); this.grid.className = 'region-pixabay-grid';
      const actions = document.createElement('div'); actions.className = 'region-pixabay-actions';
      this.importButton = this.action('导入所选', () => this.import(false), true);
      this.allButton = this.action('全部导入', () => this.import(true));
      actions.append(this.importButton, this.allButton);
      this.panel.append(toolbar, this.status, this.grid, actions); host.appendChild(this.panel);
      this.refresh();
    }

    action(label, callback, primary = false) {
      const button = document.createElement('button'); button.type = 'button';
      button.className = 'btn btn-sm ' + (primary ? 'btn-primary' : 'btn-outline-secondary');
      button.dataset.adminWrite = 'regions'; button.textContent = label; button.addEventListener('click', callback); return button;
    }

    pause() {
      this.generation++; this.controller?.abort(); this.controller = null; this.loading = false;
      if (this.status) this.refresh();
    }

    eligible(candidate) {
      return !this.options.editor().items.some(item => item.photo.id === 'pixabay-' + candidate.id || item.photo.source_url === candidate.source_url);
    }

    refresh() {
      const editable = this.options.editable() && !this.options.locked();
      const candidates = this.result?.candidates || [];
      this.button.disabled = !editable;
      this.input.disabled = this.loading || !editable;
      this.searchButton.disabled = this.loading || !editable;
      this.searchButton.textContent = this.loading ? '查询中…' : '查询';
      this.nextButton.disabled = this.loading || !editable || !this.result?.has_more;
      const room = 20 - this.options.editor().items.length;
      this.importButton.disabled = this.loading || !editable || room <= 0 || !candidates.some(candidate => this.selected.has(candidate.id) && this.eligible(candidate));
      this.allButton.disabled = this.loading || !editable || room <= 0 || !candidates.some(candidate => this.eligible(candidate));
      this.status.textContent = this.loading ? '正在查询…' : this.message;
      this.grid.querySelectorAll('input[type=checkbox]').forEach(input => {
        const candidate = candidates.find(item => item.id === input.value);
        const eligible = candidate && this.eligible(candidate);
        input.disabled = this.loading || !editable || !eligible || room <= 0;
        input.closest('.region-pixabay-item').classList.toggle('is-imported', !eligible);
        const text = input.closest('.region-pixabay-item').querySelector('.region-pixabay-selection');
        text.textContent = eligible ? '选择此图' : '已加入';
      });
    }

    renderCandidates() {
      this.grid.replaceChildren();
      (this.result?.candidates || []).forEach(candidate => {
        const card = document.createElement('div'); card.className = 'region-pixabay-item';
        const choice = document.createElement('label'); choice.className = 'region-pixabay-choice';
        const image = document.createElement('img'); image.src = candidate.preview_url; image.alt = candidate.tags || '候选图片';
        image.loading = 'lazy'; image.referrerPolicy = 'no-referrer'; image.dataset.noPreview = '';
        const line = document.createElement('span'); line.className = 'region-pixabay-check';
        const checkbox = document.createElement('input'); checkbox.type = 'checkbox'; checkbox.className = 'form-check-input'; checkbox.value = candidate.id;
        checkbox.setAttribute('aria-label', '选择候选图片 ' + candidate.id);
        checkbox.addEventListener('change', () => { if (checkbox.checked) this.selected.add(candidate.id); else this.selected.delete(candidate.id); this.refresh(); });
        const text = document.createElement('span'); text.className = 'region-pixabay-selection';
        line.append(checkbox, text); choice.append(image, line);
        const source = document.createElement('a'); source.href = candidate.source_url; source.target = '_blank'; source.rel = 'noopener noreferrer';
        source.textContent = candidate.author || '查看原图'; source.title = '在 Pixabay 查看来源';
        card.append(choice, source); this.grid.appendChild(card);
      });
      this.refresh();
    }

    async search(page) {
      if (this.loading || !this.options.editable() || this.options.locked()) return;
      const query = this.input.value.trim();
      if (!query) { this.message = '请填写查询关键词'; this.refresh(); return; }
      if (page !== 1 && query !== this.result?.query) page = 1;
      this.pause();
      const generation = this.generation;
      const controller = new AbortController(); this.controller = controller;
      const timeout = setTimeout(() => controller.abort(), 45000);
      this.loading = true; this.refresh();
      try {
        const response = await fetch('/admin/api/regions/photos/pixabay/search', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ query, page }), signal: controller.signal
        });
        const body = await response.json();
        if (generation !== this.generation) return;
        if (!response.ok || body.code !== 0) throw new Error(body.error || '查询失败，请重试');
        this.result = body.data; this.selected.clear();
        this.message = this.result.candidates.length ? '确认图片属于当前地区后再导入。' : '没有找到图片，试试英文名或具体地标。';
        this.renderCandidates();
      } catch (error) {
        if (generation === this.generation) this.message = error.name === 'AbortError' ? '查询超时，请重试' : error.message;
      } finally {
        clearTimeout(timeout);
        if (generation === this.generation) { this.loading = false; this.controller = null; this.refresh(); }
      }
    }

    import(all) {
      if (!this.options.editable() || this.options.locked() || this.loading || !this.result) return;
      const candidates = this.result.candidates.filter(candidate => (all || this.selected.has(candidate.id)) && this.eligible(candidate));
      const added = this.options.editor().addRemotePhotos(candidates, this.result.query, this.result.page) || 0;
      this.selected.clear(); this.grid.querySelectorAll('input[type=checkbox]').forEach(input => { input.checked = false; });
      this.message = added ? '已加入' + added + '张图片' + (added < candidates.length ? '，其余超出20张上限' : '') + '，完成后请保存当前节点。' : '未加入新图片，请检查数量或是否重复。';
      this.refresh();
    }
  };
})();
