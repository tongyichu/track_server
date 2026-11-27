/* Track photo gallery, moderation and map previews. */
const AdminTrackPhotos = (() => {
  const {node, button, tile, time} = WorkoutPhotos;
  const states = {pending: '待审核', approved: '已通过', rejected: '已驳回'};

  function create(trackID, signal, onPhotos) {
    const base = '/admin/api/tracks/' + encodeURIComponent(trackID) + '/photos';
    const imageURL = p => base + '/' + encodeURIComponent(p.id) + '/image';
    const section = node('section', 'admin-detail-photos');
    const title = node('h3', '', '用户随拍');
    const body = node('div', '', '正在加载照片…');
    const message = node('p');
    message.setAttribute('role', 'status');
    section.append(title, body, message);
    const canReview = () => AdminAccess.can('track_submissions', true);
    let photos = [], viewer, active, busy = false;

    function actions(p) {
      if (!canReview()) return [];
      const buttons = [];
      if (p.status !== 'approved') buttons.push(button('通过', 'btn btn-sm btn-outline-primary', () => decide(p, 'approved')));
      if (p.status !== 'rejected') buttons.push(button('驳回', 'btn btn-sm btn-outline-danger', () => reject(p)));
      for (const b of buttons) {
        b.dataset.adminWrite = 'track_submissions';
        b.disabled = busy;
      }
      return buttons;
    }

    async function reject(p) {
      if (busy || signal.aborted || !canReview()) return;
      const form = await AdminUI.form({
        title:'驳回照片', confirmText:'确认驳回', waitForHidden:true,
        fields:[{name:'reason', type:'textarea', label:'驳回原因', required:true, maxLength:250}],
        onSubmit(values) { if (!values.reason.trim()) throw new Error('请填写驳回原因'); }
      });
      if (form) await decide(p, 'rejected', form.reason.trim());
    }

    function caption(p) {
      const content = node('figcaption');
      const state = node('span', 'admin-photo-state', states[p.status]);
      state.dataset.status = p.status;
      content.append(state);
      if (p.description) content.append(node('span', 'admin-photo-description', p.description));
      if (p.status === 'rejected' && p.reason) content.append(node('span', '', '驳回原因：' + p.reason));
      if (p.status === 'pending' && canReview()) {
        const row = node('div', 'admin-photo-review-actions');
        row.append(...actions(p)); content.append(row);
      }
      return content;
    }

    async function decide(p, status, reason = '') {
      if (busy || signal.aborted || !photos.includes(p) || !canReview()) return;
      busy = true;
      message.textContent = '';
      section.querySelectorAll('[data-admin-write]').forEach(b => { b.disabled = true; });
      if (viewer?.open) preview(active.items, active.index);
      try {
        const response = await WorkoutPhotos.review(p, status, reason, signal);
        const result = await response.json().catch(() => ({}));
        if (!response.ok) {
          throw new Error(response.status === 409 ? '照片已被其他人更新，请核对最新状态' : result.error || '审核失败');
        }
        if (result.code !== 0 || result.data !== true) throw new Error('审核结果待核实，请查看最新状态');
        if (signal.aborted) return;
        Object.assign(p, {status, reason, revision:p.revision + 1});
        const card = body.querySelector('[data-photo-id="' + CSS.escape(p.id) + '"]');
        const restoreFocus = card.contains(document.activeElement);
        card.querySelector('figcaption').replaceWith(caption(p));
        if (restoreFocus) card.querySelector('.photo-thumb').focus({preventScroll:true});
        message.textContent = '照片' + states[status];
      } catch (error) {
        if (signal.aborted) return;
        viewer?.close();
        message.textContent = error instanceof TypeError ? '审核结果待核实，请查看最新状态' : error.message;
        await load();
      } finally {
        busy = false;
        if (!signal.aborted) {
          section.querySelectorAll('[data-admin-write]').forEach(b => { b.disabled = false; });
          if (viewer?.open) preview(active.items, active.index);
        }
      }
    }

    function preview(items, index) {
      if (signal.aborted) return;
      const p = items[index];
      active = {items, index};
      viewer ||= WorkoutPhotos.viewer();
      viewer.render({url:imageURL(p), title:states[p.status] || '用户随拍',
        detail:[time(p), p.description, p.reason].filter(Boolean).join(' · '),
        index, count:items.length, move:i => preview(items, i), actions:actions(p), busy});
    }

    function popup(items, close) {
      const box = node('div', 'admin-photo-popup');
      const dismiss = button('×', 'admin-photo-dismiss', close);
      dismiss.setAttribute('aria-label', '关闭随拍浮窗');
      const photo = button('', 'admin-photo-popup-image', () => preview(items, index));
      photo.setAttribute('data-no-preview', '');
      photo.setAttribute('aria-label', '查看照片大图');
      const image = new Image();
      image.alt = '运动随拍';
      photo.append(image);
      const caption = node('div', 'admin-photo-caption');
      box.append(photo, dismiss, caption);
      let index = 0;
      const previous = button('‹', 'btn btn-sm btn-link', () => show(index - 1));
      const next = button('›', 'btn btn-sm btn-link', () => show(index + 1));
      previous.setAttribute('aria-label', '上一张随拍'); next.setAttribute('aria-label', '下一张随拍');
      const count = node('span');
      if (items.length > 1) {
        const nav = node('div', 'admin-photo-point-nav');
        nav.append(previous, count, next); box.append(nav);
      }
      function show(i) {
        index = i;
        const p = items[i];
        image.src = imageURL(p);
        caption.textContent = p.description || '';
        caption.hidden = !p.description;
        count.textContent = (i + 1) + ' / ' + items.length;
        previous.disabled = i === 0; next.disabled = i === items.length - 1;
      }
      box.addEventListener('click', event => event.stopPropagation());
      show(0);
      return box;
    }

    async function load() {
      body.textContent = '正在加载照片…';
      try {
        const response = await fetch(base, {signal});
        if (!response.ok) throw new Error('照片加载失败');
        const {data} = await response.json();
        if (signal.aborted) return;
        if (!Array.isArray(data)) throw new Error('照片数据无效');
        photos = data;
      } catch (error) {
        if (!signal.aborted) {
          photos = [];
          onPhotos(photos);
          title.textContent = '用户随拍';
          body.replaceChildren(document.createTextNode('照片加载失败 '), button('重试', 'btn btn-sm btn-link', load));
        }
        return;
      }
      title.textContent = photos.length ? '用户随拍（' + photos.length + '）' : '用户随拍';
      if (!photos.length) {
        body.textContent = '暂无上传照片';
        onPhotos(photos);
        return;
      }
      const grid = node('div', 'admin-photo-grid');
      photos.forEach((p, i) => {
        const card = node('figure');
        card.dataset.photoId = p.id;
        card.append(tile(imageURL(p), '查看第 ' + (i + 1) + ' 张随拍', () => preview(photos, i)), caption(p));
        grid.append(card);
      });
      body.replaceChildren(grid);
      onPhotos(photos);
    }
    signal.addEventListener('abort', () => viewer?.dispose(), {once:true});
    return {section, load, popup};
  }
  return {create};
})();
