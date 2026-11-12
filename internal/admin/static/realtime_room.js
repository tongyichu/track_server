(function () {
  'use strict';
  const eventNames = { member_left: '成员离开', member_disconnected: '成员断线', member_reconnected: '成员重连', notice_sent: '同行周知', checkpoint_reached: '到达关键点', risk_reported: '风险提醒', custom: '自定义事件' };
  function compareTime(a, b) {
    const difference = Date.parse(a) - Date.parse(b); if (difference) return difference;
    const fraction = value => ((/\.(\d+)/.exec(value)?.[1] || '').padEnd(9, '0')).slice(3, 9);
    return fraction(a).localeCompare(fraction(b));
  }
  function el(tag, text, className) { const node = document.createElement(tag); if (text != null) node.textContent = String(text); if (className) node.className = className; return node; }
  function button(text, action) { const node = el('button', text, 'btn btn-sm btn-outline-secondary'); node.type = 'button'; node.addEventListener('click', action); return node; }
  function mount({ host, sessionID, userID = '', api, onTitle }) {
    const panel = el('section', null, 'rt-chat-panel rt-room-panel'), heading = el('div', null, 'rt-chat-heading'), identity = el('div', null, 'rt-chat-identity');
    const refreshButton = button('刷新', () => refresh()); heading.append(identity, refreshButton);
    const feed = el('div', null, 'rt-chat-feed'); feed.tabIndex = 0; feed.setAttribute('aria-label', userID ? '本房间内的个人事件列表，向上翻阅已加载记录' : '同行群聊和事件，向上翻阅已加载记录');
    const scope = el('p', '正在加载房间记录…', 'rt-room-scope text-secondary'), list = el('div'); feed.append(scope, list);
    const status = el('span', '', 'text-secondary small'); status.setAttribute('role', 'status');
    const newest = button('回到最新', bottom); newest.hidden = true;
    const footer = el('div', null, 'rt-chat-footer'); footer.append(status, newest);
    const note = el('p', '只读查看存储记录；关键事件由房主上报，不代表 Broker 完整收发历史。', 'rt-chat-note text-secondary');
    panel.append(heading, feed, footer, note);
    const summary = el('details', null, 'rt-room-summary'), summaryTitle = el('summary', '房间信息与位置快照'), summaryBody = el('div'); summary.append(summaryTitle, summaryBody);
    host.append(panel, summary);
    let request, version = 0, busy = false, disposed = false, ready = false, headingKey = '', summaryKey = '';
    const rows = new Map();
    function bottom() { feed.scrollTop = feed.scrollHeight; newest.hidden = true; }
    function person(data, id) { return data.people[id] || { id, nickname: '用户 ' + id, avatar_url: '' }; }
    function stop() { request?.abort(); version++; busy = false; refreshButton.disabled = false; }
    function timestamp(at) { const time = el('time', AdminTime.dateTime(at), 'rt-dialog-time text-secondary'); time.dateTime = at; return time; }
    function messageRow(record, data) {
      const message = record.value, author = person(data, message.sender_id), row = el('article', null, userID ? 'rt-room-personal' : 'rt-dialog-message');
      row.append(timestamp(record.at));
      if (userID) {
        row.append(el('strong', '发送弹幕', 'rt-room-event-title'), el('p', message.content, 'rt-room-text')); return row;
      }
      const line = el('div', null, 'rt-dialog-row'), body = el('div', null, 'rt-dialog-body'), bubble = el('div', null, 'rt-dialog-bubble');
      const name = el('span', author.nickname + (message.sender_id === data.room.owner_id ? ' · 房主' : ''), 'rt-dialog-name text-secondary'); name.title = message.sender_id;
      bubble.append(el('p', message.content)); bubble.title = AdminTime.dateTime(record.at);
      body.append(name, bubble); line.append(RealtimeChat.avatar(author), body); row.append(line); return row;
    }
    function eventRow(record, data) {
      const event = record.value, row = el('article', null, userID ? 'rt-room-personal' : 'rt-room-system'); row.append(timestamp(record.at));
      const card = el('div', null, 'rt-room-event-card');
      card.append(el('span', eventNames[event.event_type] || '同行事件', 'badge bg-blue-lt'));
      if (event.target_user_id) card.append(el('strong', person(data, event.target_user_id).nickname, 'rt-room-event-target'));
      if (event.title) card.append(el('strong', event.title, 'rt-room-event-title'));
      if (event.content) card.append(el('p', event.content, 'rt-room-text'));
      card.append(el('small', '房主上报 · ' + person(data, event.owner_user_id).nickname, 'text-secondary')); row.append(card); return row;
    }
    function renderSummary(data) {
      const key = JSON.stringify([data.room, data.members, data.positions, data.people]); if (key === summaryKey) return; summaryKey = key;
      const fragment = document.createDocumentFragment();
      fragment.append(el('p', data.room.session_id, 'text-secondary small'), el('p', '开始：' + AdminTime.dateTime(data.room.started_at), 'text-secondary small'));
      for (const member of data.members) {
        if (userID && member.user_id !== userID) continue;
        const current = person(data, member.user_id), position = data.positions[member.user_id], row = el('div', null, 'rt-network-member');
        const state = { joined: '已加入', left: '已离开', kicked: '已移出', ended: '已结束' }[member.member_status] || member.member_status;
        row.append(el('strong', current.nickname + (member.role === 'owner' ? ' · 房主' : '')), el('small', member.user_id, 'text-secondary'), el('small', state + ' · ' + (member.connections ? member.connections + ' 条在线连接' : '未采样到在线连接'), 'text-secondary'));
        row.append(el('small', position ? '最新位置快照：' + position.coordinate_system + ' · ' + Number(position.latitude).toFixed(5) + ', ' + Number(position.longitude).toFixed(5) + ' · ' + AdminTime.dateTime(position.recorded_at) : '暂无位置快照', 'text-secondary'));
        fragment.append(row);
      }
      summaryBody.replaceChildren(fragment);
    }
    function render(data) {
      const current = userID ? person(data, userID) : null, roomTitle = data.room.title || '同行房间';
      onTitle(userID ? current.nickname + ' · 个人事件' : roomTitle + ' · 群聊与事件');
      const key = JSON.stringify([roomTitle, current, data.members.length]);
      if (key !== headingKey) {
        const text = el('div'); text.append(el('strong', userID ? current.nickname : roomTitle), el('small', userID ? '本房间内与此人相关的记录' : data.members.length + ' 位成员 · 群聊与关键事件', 'text-secondary'));
        identity.replaceChildren(); if (current) identity.append(RealtimeChat.avatar(current)); identity.append(text); headingKey = key;
      }
      const records = [];
      for (const message of data.messages) if (!userID || message.sender_id === userID) records.push({ id: 'message:' + message.id, kind: 'message', at: message.created_at, value: message });
      for (const event of data.events) if (!userID || event.target_user_id === userID || !event.target_user_id && event.owner_user_id === userID) records.push({ id: 'event:' + event.id, kind: 'event', at: event.event_time, value: event });
      records.sort((a, b) => compareTime(a.at, b.at) || a.id.localeCompare(b.id, 'en', { numeric: true }));
      const pinned = feed.scrollHeight - feed.scrollTop - feed.clientHeight < 64, bounds = feed.getBoundingClientRect();
      const anchor = [...list.children].find(row => row.getBoundingClientRect().bottom > bounds.top && row.getBoundingClientRect().top < bounds.bottom);
      const anchorID = anchor?.dataset.recordId, before = anchor?.getBoundingClientRect().top, wanted = new Set(records.map(record => record.id));
      let inserted = 0;
      for (const [id, entry] of rows) if (!wanted.has(id)) { entry.row.remove(); rows.delete(id); }
      if (!rows.size) list.replaceChildren();
      for (let index = 0; index < records.length; index++) {
        const record = records[index], authorID = record.kind === 'message' ? record.value.sender_id : record.value.owner_user_id;
        const fingerprint = JSON.stringify([record, person(data, authorID), data.people[record.value.target_user_id], data.room.owner_id]);
        let entry = rows.get(record.id);
        if (!entry || entry.key !== fingerprint) {
          const row = record.kind === 'message' ? messageRow(record, data) : eventRow(record, data); row.dataset.recordId = record.id;
          if (entry) entry.row.replaceWith(row); else inserted++;
          entry = { key: fingerprint, row }; rows.set(record.id, entry);
        }
        if (list.children[index] !== entry.row) list.insertBefore(entry.row, list.children[index] || null);
        const previous = records[index - 1], time = entry.row.querySelector('time');
        const gap = previous ? Date.parse(record.at) - Date.parse(previous.at) : NaN;
        time.hidden = !userID && Number.isFinite(gap) && gap >= 0 && gap < 5 * 60 * 1000;
      }
      if (!records.length) list.replaceChildren(el('p', userID ? '已加载的房间记录中暂无此人的事件或弹幕' : '暂无存储弹幕或关键事件', 'rt-chat-placeholder text-secondary'));
      scope.textContent = '本次载入最近' + data.messages.length + '条弹幕、' + data.events.length + '条关键事件' + (data.messages_limited || data.events_limited ? '，较早记录未全部展示' : '') + (userID ? '；仅筛选与此人相关的记录' : '');
      renderSummary(data);
      if (!ready || pinned) {
        bottom();
        if (!ready) { const epoch = version; requestAnimationFrame(() => { if (!disposed && epoch === version) bottom(); }); }
      } else if (anchorID && rows.has(anchorID)) feed.scrollTop += rows.get(anchorID).row.getBoundingClientRect().top - before;
      if (ready && inserted && !pinned) newest.hidden = false;
      ready = true;
      status.className = 'text-secondary small'; status.textContent = '采样：' + AdminTime.dateTime(data.sampled_at) + (data.status === 'stale' ? '（上次采样）' : '');
    }
    async function refresh() {
      if (busy || disposed || document.hidden) return;
      busy = true; refreshButton.disabled = true; const epoch = ++version, task = new AbortController(); request = task;
      let timedOut = false; const timeout = setTimeout(() => { timedOut = true; task.abort(); }, 10000);
      try {
        const data = await api('network/room?' + new URLSearchParams({ session_id: sessionID }), task.signal);
        if (disposed || task.signal.aborted || epoch !== version) return;
        render(data);
      } catch (error) {
        if (!disposed && epoch === version && (timedOut || !task.signal.aborted)) {
          status.className = 'text-danger small'; status.textContent = (timedOut ? '房间查询超时' : error.message) + (ready ? '，保留上次内容' : '');
          if (!ready) scope.textContent = '房间记录加载失败，可点击刷新重试';
        }
      } finally { clearTimeout(timeout); if (epoch === version) { busy = false; refreshButton.disabled = false; } }
    }
    feed.addEventListener('scroll', () => { if (feed.scrollHeight - feed.scrollTop - feed.clientHeight < 64) newest.hidden = true; }, { passive: true });
    refresh(); return { type: 'room', sessionID, userID, refresh, stop, dispose() { disposed = true; stop(); } };
  }
  window.RealtimeRoom = { mount };
}());
