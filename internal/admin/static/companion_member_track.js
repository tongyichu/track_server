(function () {
  'use strict';
  function cell(track, onOpen) {
    const td = document.createElement('td');
    td.className = 'companion-member-track';
    td.setAttribute('data-admin-read', 'tracks');
    const content = document.createElement('div');
    content.className = 'companion-cell-stack';
    td.appendChild(content);
    if (track && track.track_id && ['ready', 'deleted'].includes(track.state)) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'btn btn-link p-0 text-start text-wrap';
      button.textContent = track.title || track.track_id;
      button.addEventListener('click', function () { onOpen(track.track_id, button); });
      content.appendChild(button);
      if (track.title) {
        const id = document.createElement('small');
        id.className = 'd-block text-secondary'; id.textContent = track.track_id;
        content.appendChild(id);
      }
      if (track.state === 'deleted') {
        const badge = document.createElement('span');
        badge.className = 'badge bg-secondary-lt'; badge.textContent = '已下架';
        content.appendChild(badge);
      }
    } else {
      const labels = { pending: '尚未上传', empty: '无有效轨迹', unavailable: '轨迹暂不可用', unknown: '读取失败' };
      content.classList.add('text-secondary');
      content.textContent = track ? labels[track.state] || '状态未知' : '无查看权限';
    }
    return td;
  }
  window.AdminCompanionMemberTrack = { cell: cell };
})();
