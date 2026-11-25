/* Shared by the moderation queue and scenic photo gallery. */
const WorkoutPhotos = (() => {
  function node(tag, className, text) {
    const element = document.createElement(tag);
    if (className) element.className = className;
    if (text != null) element.textContent = text;
    return element;
  }
  function button(label, style, click) {
    const element = node('button', style, label);
    element.type = 'button';
    element.onclick = click;
    return element;
  }
  function time(photo) {
    return AdminTime.dateTime(photo.taken_at_ms);
  }
  function tile(url, label, open) {
    const thumb = button('', 'photo-thumb', open);
    thumb.setAttribute('aria-label', label);
    const img = new Image();
    img.loading = 'lazy';
    img.alt = '';
    img.src = url;
    thumb.append(img);
    return thumb;
  }
  function viewer() {
    const dialog = node('dialog', 'photo-viewer');
    dialog.setAttribute('aria-label', '照片详情');
    dialog.innerHTML = `<header><div><strong></strong><small></small></div><button type="button" class="btn-close" aria-label="关闭"></button></header>
      <div class="photo-stage"><img alt="运动实拍大图"><span role="status"></span></div>
      <footer><div class="photo-navigation"><button type="button" aria-label="上一张">‹</button><span aria-live="polite"></span><button type="button" aria-label="下一张">›</button></div><div class="photo-actions"></div></footer>`;
    document.body.append(dialog);
    const find = selector => dialog.querySelector(selector);
    const prev = find('[aria-label="上一张"]'), next = find('[aria-label="下一张"]');
    const image = find('img'), message = find('.photo-stage span');
    function clearImage() { image.removeAttribute('src'); message.textContent = ''; }
    find('.btn-close').onclick = () => dialog.close();
    dialog.addEventListener('close', () => { if (!dialog.open) clearImage(); });
    image.onerror = () => { if (dialog.open) message.textContent = '图片加载失败'; };
    dialog.onkeydown = event => {
      if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
        event.preventDefault();
        (event.key === 'ArrowLeft' ? prev : next).click();
      }
    };
    return {
      get open() { return dialog.open; },
      close() { if (dialog.open) dialog.close(); },
      render({url, title, detail, index, count, move, actions = [], busy = false}) {
        if (image.getAttribute('src') !== url) {
          message.textContent = '';
          image.src = url;
        }
        find('strong').textContent = title;
        find('small').textContent = detail;
        find('.photo-navigation span').textContent = (index + 1) + ' / ' + count;
        prev.onclick = () => move(index - 1);
        next.onclick = () => move(index + 1);
        prev.disabled = busy || index === 0;
        next.disabled = busy || index >= count - 1;
        const actionBar = find('.photo-actions');
        const focused = actionBar.contains(document.activeElement) ? document.activeElement.textContent : '';
        actionBar.replaceChildren(...actions);
        actions.forEach(action => { action.disabled = busy; });
        if (!dialog.open) dialog.showModal();
        if (focused && !busy) actions.find(action => action.textContent === focused)?.focus();
      }
    };
  }
  return {node, button, time, tile, viewer};
})();
