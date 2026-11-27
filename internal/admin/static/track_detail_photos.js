/* Uploaded photos and their read-only previews in the shared track editor. */
const AdminTrackPhotos = (() => {
  const {node, button, tile, time} = WorkoutPhotos;
  const states = {pending: '待审核', approved: '已通过', rejected: '已驳回'};

  function create(trackID, signal, onPhotos) {
    const base = '/admin/api/tracks/' + encodeURIComponent(trackID) + '/photos';
    const imageURL = p => base + '/' + encodeURIComponent(p.id) + '/image';
    const section = node('section', 'admin-detail-photos');
    const title = node('h3', '', '用户随拍');
    const body = node('div', '', '正在加载照片…');
    section.append(title, body);
    let viewer;

    function preview(items, index) {
      if (signal.aborted) return;
      const p = items[index];
      viewer ||= WorkoutPhotos.viewer();
      viewer.render({url:imageURL(p), title:states[p.status] || '用户随拍',
        detail:[time(p), p.description, p.reason].filter(Boolean).join(' · '),
        index, count:items.length, move:i => preview(items, i)});
    }

    function popup(items, close) {
      const box = node('div', 'admin-photo-popup');
      const dismiss = button('×', 'admin-photo-dismiss', close);
      dismiss.setAttribute('aria-label', '关闭随拍浮窗');
      const photo = button('', 'admin-photo-popup-image', () => preview(items, index));
      photo.setAttribute('aria-label', '查看照片大图');
      const image = new Image();
      image.alt = '运动随拍';
      photo.append(image);
      const caption = node('div', 'admin-photo-caption');
      box.append(photo, dismiss, caption);
      let index = 0;
      const previous = button('‹', 'btn btn-sm', () => show(index - 1));
      const next = button('›', 'btn btn-sm', () => show(index + 1));
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
      let photos;
      try {
        const response = await fetch(base, {signal});
        if (!response.ok) throw new Error('照片加载失败');
        const {data} = await response.json();
        if (signal.aborted) return;
        if (!Array.isArray(data)) throw new Error('照片数据无效');
        photos = data;
      } catch (error) {
        if (!signal.aborted) body.replaceChildren(document.createTextNode('照片加载失败 '), button('重试', 'btn btn-sm btn-link', load));
        return;
      }
      title.textContent = photos.length ? '用户随拍（' + photos.length + '）' : '用户随拍';
      if (!photos.length) {
        body.textContent = '暂无上传照片';
        return;
      }
      const strip = node('div', 'admin-photo-strip');
      photos.forEach((p, i) => {
        const card = node('figure');
        const caption = node('figcaption', '', states[p.status] || '');
        if (p.description) caption.append(node('span', 'admin-photo-description', p.description));
        card.append(tile(imageURL(p), '查看第 ' + (i + 1) + ' 张随拍', () => preview(photos, i)), caption);
        strip.append(card);
      });
      body.replaceChildren(strip);
      onPhotos(photos);
    }
    signal.addEventListener('abort', () => viewer?.dispose(), {once:true});
    return {section, load, popup};
  }
  return {create};
})();
