(function () {
  'use strict';
  window.RegionPhotosEditor = class {
    constructor(root, options) {
      this.root = root;
      this.options = options;
      this.sessions = new Map();
      this.node = null;
      window.addEventListener('pagehide', () => this.sessions.forEach(session => session.editor.dispose()));
    }
    get editor() { return this.sessions.get(this.node?.id)?.editor; }
    get busy() { return this.editor?.busy || false; }
    get failed() { return this.editor?.failed || false; }
    get pending() { return this.editor?.pending || false; }
    settle() { return this.editor?.settle() || Promise.resolve(); }

    bind(node) {
      this.node = node;
      this.root.replaceChildren();
      this.root.hidden = !node || !['province', 'city'].includes(node.level);
      if (this.root.hidden) return;
      const title = document.createElement('h3'); title.textContent = '宣传图片';
      const hint = document.createElement('p'); hint.className = 'form-hint';
      hint.textContent = '可拖动或使用箭头调整顺序，图片信息随当前节点保存。';
      let session = this.sessions.get(node.id);
      if (!session) {
        session = { node, host: document.createElement('div') };
        const editor = new AdminPhotoEditor(session.host, {
          module: 'regions', label: '宣传图片', max: 20,
          editable: this.options.editable,
          uploadURL: () => this.options.saved(session.node) ? '/admin/api/regions/' + encodeURIComponent(session.node.id) + '/photos' : null,
          photoURL: url => url.startsWith('/api/v1/static/') ? url.replace('/api/v1/static/', '/admin/api/static/') : url,
          changed: () => {
            session.node.photos = editor.photos;
            this.options.changed(session.node);
          },
          stateChanged: this.options.busyChanged
        });
        session.editor = editor;
        editor.load(node.photos);
        this.sessions.set(node.id, session);
      }
      if (session.node !== node && !session.editor.pending) session.editor.load(node.photos);
      session.node = node;
      this.root.append(title, hint, session.host);
      session.editor.start();
    }
  };
})();
