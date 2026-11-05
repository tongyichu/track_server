(function () {
  'use strict';
  window.RegionPhotosEditor = class {
    constructor(root, options) {
      this.root = root;
      this.options = options;
      this.sessions = new Map();
      this.node = null;
      window.addEventListener('pagehide', () => this.sessions.forEach(session => { session.picker.pause(); session.editor.dispose(); }));
    }
    get editor() { return this.sessions.get(this.node?.id)?.editor; }
    get busy() { return this.editor?.busy || false; }
    get failed() { return this.editor?.failed || false; }
    get pending() { return Array.from(this.sessions.values()).some(session => session.editor.pending); }
    settle() { return this.editor?.settle() || Promise.resolve(); }

    remove(ids) {
      ids.forEach(id => {
        const session = this.sessions.get(id);
        if (!session) return;
        session.picker.pause();
        session.editor.dispose();
        this.sessions.delete(id);
      });
    }

    bind(node) {
      this.sessions.get(this.node?.id)?.picker.pause();
      this.sessions.get(this.node?.id)?.editor.pauseInfo();
      this.node = node;
      this.root.replaceChildren();
      this.root.hidden = !node || !['province', 'city'].includes(node.level);
      if (this.root.hidden) return;
      const title = document.createElement('h3'); title.textContent = '宣传图片';
      const heading = document.createElement('div'); heading.className = 'region-photos-heading'; heading.appendChild(title);
      const hint = document.createElement('p'); hint.className = 'form-hint';
      hint.textContent = '可拖动调整顺序。';
      let session = this.sessions.get(node.id);
      if (!session) {
        session = { node, host: document.createElement('div'), pickerHost: document.createElement('div') };
        const editor = new AdminPhotoEditor(session.host, {
          module: 'regions', label: '宣传图片', max: 20,
          editable: this.options.editable,
          ready: () => this.options.saved(session.node),
          uploadURL: () => this.options.saved(session.node) ? '/admin/api/regions/' + encodeURIComponent(session.node.id) + '/photos' : null,
          importURL: () => this.options.saved(session.node) ? '/admin/api/regions/' + encodeURIComponent(session.node.id) + '/photos/pixabay/import' : null,
          photoURL: url => url.startsWith('/api/v1/static/') ? url.replace('/api/v1/static/', '/admin/api/static/') : url,
          changed: () => {
            session.picker?.refresh();
          },
          savePhotos: (photos, base, signal) => this.options.savePhotos(session.node, photos, base, signal),
          photosSaved: (result, photos) => this.options.photosSaved(session.node, result, photos),
          stateChanged: () => { this.options.busyChanged(); session.picker?.refresh(); }
        });
        session.editor = editor;
        session.picker = new RegionPixabayPicker(session.pickerHost, {
          node: () => session.node, editor: () => editor, editable: () => this.options.editable() && this.options.saved(session.node),
          locked: this.options.locked
        });
        editor.load(node.photos);
        this.sessions.set(node.id, session);
      }
      if (session.node !== node && !session.editor.pending) session.editor.load(node.photos);
      session.node = node;
      heading.appendChild(session.picker.button);
      this.root.append(heading, hint, session.pickerHost, session.host);
      session.picker.refresh();
      session.editor.start();
    }
  };
})();
