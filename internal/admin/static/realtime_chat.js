(function () {
  'use strict';
  const el = (tag, text, className) => {
    const result = document.createElement(tag);
    if (text != null) result.textContent = String(text);
    if (className) result.className = className;
    return result;
  };
  function button(text, className) { const result = el('button', text, className || 'btn btn-sm btn-outline-secondary'); result.type = 'button'; return result; }
  function avatar(person) {
    const root = el('span', (person.nickname || '用').slice(0, 1), 'rt-chat-avatar');
    const url = person.avatar_url || '';
    if (url.startsWith('/admin/api/static/avatars/') || url.startsWith('/admin/api/static/default_avatars/')) {
      const image = el('img'); image.alt = person.nickname || ''; image.width = 38; image.height = 38;
      image.dataset.noPreview = ''; image.addEventListener('error', () => image.remove(), { once: true }); image.src = url; root.append(image);
    }
    return root;
  }
  function mount({ host, sidebar, item, signal, api, onMetadata }) {
    const contactPanel = el('section', null, 'rt-chat-contacts');
    const allContacts = button('全部私信', 'btn btn-sm btn-outline-secondary rt-stream-all'), contacts = el('div', null, 'rt-chat-contact-list'), moreContacts = button('更多联系人'), contactsError = el('p', '', 'text-danger small');
    contactsError.setAttribute('role', 'status');
    contactPanel.append(el('h3', '按联系人筛选', 'realtime-detail-title'), allContacts, contacts, contactsError, moreContacts); sidebar.append(contactPanel);
    const heading = el('div', null, 'rt-chat-heading'), identity = el('div', null, 'rt-chat-identity'), refresh = button('刷新消息');
    heading.append(identity, refresh);
    const feed = el('div', null, 'rt-chat-feed'); feed.tabIndex = 0; feed.setAttribute('aria-label', '账号私信消息流，向上翻阅历史记录');
    const older = button('下拉查看更早消息', 'btn btn-sm btn-outline-secondary rt-chat-history'), messages = el('div', null, 'rt-chat-messages');
    const placeholder = el('p', '正在加载消息流…', 'rt-chat-placeholder text-secondary'); messages.append(placeholder); feed.append(older, messages);
    const status = el('span', '', 'text-secondary small'), newest = button('回到最新消息'); newest.hidden = true;
    const footer = el('div', null, 'rt-chat-footer'); footer.append(status, newest);
    host.append(heading, feed, footer, el('p', '只读查看账号全部设备的存储私信；查看不会标记已读。', 'text-secondary small rt-chat-note'));
    const people = new Map(), conversations = new Map(), contactRows = new Map(), messageRows = new Map();
    const active = new Map(); let peer = '', latestSeq = '0', latestCursor = 'origin', nextCursor = '', hasOlder = false, nextContacts = '', hasContacts = false;
    let initialized = false, initializing = false, chatReady = false, chatBusy = false, contactsBusy = false, selection = 0, lifecycle = 0, disposed = false, headingKey = null;
    const person = id => people.get(id) || { id, nickname: '用户 ' + id, avatar_url: '' };
    function controls() { refresh.disabled = initializing || chatBusy; older.disabled = !chatReady || chatBusy || !hasOlder; moreContacts.disabled = initializing || contactsBusy || !hasContacts; }
    function stop() { for (const entry of active.values()) entry.controller.abort(); active.clear(); selection++; lifecycle++; initializing = false; chatBusy = false; contactsBusy = false; controls(); }
    signal.addEventListener('abort', () => { disposed = true; stop(); }, { once: true });
    async function request(key, path) {
      active.get(key)?.controller.abort();
      const entry = { controller: new AbortController(), timedOut: false }; active.set(key, entry);
      const timeout = setTimeout(() => { entry.timedOut = true; entry.controller.abort(); }, 10000);
      try {
        const data = await api(path, entry.controller.signal);
        if (disposed || signal.aborted || entry.controller.signal.aborted || active.get(key) !== entry) return null;
        return data;
      } catch (error) {
        if (entry.timedOut) throw new Error('消息查询超时');
        if (entry.controller.signal.aborted || disposed) return null;
        throw error;
      } finally { clearTimeout(timeout); if (active.get(key) === entry) active.delete(key); }
    }
    function receive(data) {
      for (const [id, value] of Object.entries(data.people || {})) people.set(id, value);
      onMetadata(data.client, data.sampled_at, data.status);
    }
    function renderHeading() {
      const key = JSON.stringify([peer, person(peer || item.user_id)]);
      if (key === headingKey) return;
      headingKey = key;
      identity.replaceChildren();
      if (!peer) {
        const owner = person(item.user_id), text = el('div');
        text.append(el('strong', '账号私信消息流'), el('small', owner.nickname + ' · 全部联系人', 'text-secondary'));
        identity.append(avatar(owner), text); return;
      }
      const current = person(peer), text = el('div'); text.append(el('strong', current.nickname), el('small', '仅筛选与此人的往来记录', 'text-secondary'));
      identity.append(avatar(current), text);
    }
    function renderContacts(page, append) {
      for (const conversation of page.items) conversations.set(conversation.peer_id, conversation);
      if (append || conversations.size <= 30) { nextContacts = page.next_cursor || ''; hasContacts = page.has_more; }
      const ordered = [...conversations.values()].sort((a, b) => new Date(b.created_at) - new Date(a.created_at) || a.peer_id.localeCompare(b.peer_id));
      if (ordered.length) contacts.querySelector(':scope > p')?.remove();
      for (let index = 0; index < ordered.length; index++) {
        const conversation = ordered[index], current = person(conversation.peer_id), key = JSON.stringify([conversation, current]);
        let record = contactRows.get(conversation.peer_id);
        if (!record || record.key !== key) {
          const row = button('', 'rt-chat-contact btn btn-outline-secondary'), text = el('span', null, 'rt-chat-contact-text');
          row.setAttribute('aria-label', '筛选与' + current.nickname + '的往来记录');
          text.append(el('strong', current.nickname), el('small', conversation.content || '暂无文本消息'));
          row.append(avatar(current), text); row.title = current.nickname + ' · ' + current.id;
          row.addEventListener('click', () => selectPeer(conversation.peer_id));
          if (record) record.row.replaceWith(row);
          record = { key, row }; contactRows.set(conversation.peer_id, record);
        }
        if (contacts.children[index] !== record.row) contacts.insertBefore(record.row, contacts.children[index] || null);
        record.row.classList.toggle('active', peer === conversation.peer_id); record.row.setAttribute('aria-pressed', String(peer === conversation.peer_id));
      }
      if (!ordered.length) contacts.replaceChildren(el('p', '暂无对话记录', 'text-secondary small'));
      allContacts.classList.toggle('active', !peer); allContacts.setAttribute('aria-pressed', String(!peer));
      moreContacts.hidden = !hasContacts; controls();
    }
    function setPeer(id) {
      peer = id || ''; selection++; chatReady = false; latestSeq = '0'; latestCursor = 'origin'; nextCursor = ''; hasOlder = false;
      messageRows.clear(); messages.replaceChildren(el('p', '正在加载消息…', 'rt-chat-placeholder text-secondary')); newest.hidden = true;
      for (const [id, record] of contactRows) { record.row.classList.toggle('active', id === peer); record.row.setAttribute('aria-pressed', String(id === peer)); }
      allContacts.classList.toggle('active', !peer); allContacts.setAttribute('aria-pressed', String(!peer));
      renderHeading(); controls();
    }
    function messageNode(message) {
      const own = message.sender_id === item.user_id;
      const article = el('article', null, 'rt-stream-message'), top = el('div', null, 'rt-stream-top');
      const direction = el('span', own ? '发出' : '收到', 'badge ' + (own ? 'bg-blue-lt' : 'bg-green-lt'));
      const time = el('time', AdminTime.dateTime(message.created_at), 'text-secondary'); time.dateTime = message.created_at; top.append(direction, time);
      const route = el('div', null, 'rt-stream-route');
      for (const [index, id] of [message.sender_id, message.receiver_id].entries()) {
        if (index) route.append(el('span', '→', 'rt-stream-arrow'));
        const current = person(id), identity = el('span', null, 'rt-stream-person'), text = el('span', null, 'rt-stream-person-text');
        text.append(el('strong', current.nickname), el('small', current.id, 'text-secondary'));
        identity.append(avatar(current), text); route.append(identity);
      }
      const content = el('div', null, 'rt-stream-content');
      content.append(el('p', message.content || (message.track_id ? '分享轨迹' : '（无文本内容）')));
      if (message.track_id) { const track = el('div', null, 'rt-chat-track'); track.append(el('strong', '分享轨迹'), el('small', message.track_id)); content.append(track); }
      article.append(top, route, content); return article;
    }
    function bottom() { feed.scrollTop = feed.scrollHeight; newest.hidden = true; }
    function mergeMessages(page, mode) {
      const pinned = feed.scrollHeight - feed.scrollTop - feed.clientHeight < 64;
      const bounds = feed.getBoundingClientRect();
      const anchor = [...messageRows.entries()].find(([, record]) => { const rect = record.row.getBoundingClientRect(); return rect.bottom > bounds.top && rect.top < bounds.bottom; });
      const before = anchor?.[1].row.getBoundingClientRect().top;
      const fragment = document.createDocumentFragment(); let inserted = 0;
      if (!chatReady || !messageRows.size) messages.replaceChildren();
      for (const message of page.items) {
        const author = person(message.sender_id), key = JSON.stringify([message, author, person(message.receiver_id)]), old = messageRows.get(message.id);
        if (old?.key === key) continue;
        const row = messageNode(message); messageRows.set(message.id, { key, row, message });
        if (old) old.row.replaceWith(row); else { fragment.append(row); inserted++; }
      }
      if (mode === 'older') messages.prepend(fragment); else messages.append(fragment);
      if (mode !== 'newer') { nextCursor = page.next_cursor || ''; hasOlder = page.has_more; }
      if (mode !== 'older') { if (page.latest_seq) latestSeq = page.latest_seq; if (page.latest_cursor) latestCursor = page.latest_cursor; }
      chatReady = true; older.textContent = hasOlder ? '下拉查看更早消息' : '已到最早消息';
      if (!messageRows.size) messages.replaceChildren(el('p', '暂无存储消息', 'rt-chat-placeholder text-secondary'));
      if (mode === 'initial' || mode === 'newer' && pinned) {
        bottom();
        if (mode === 'initial') {
          const version = selection;
          requestAnimationFrame(() => { if (!disposed && version === selection) bottom(); });
        }
      }
      else if (anchor) {
        const row = messageRows.get(anchor[0])?.row;
        if (row?.isConnected) feed.scrollTop += row.getBoundingClientRect().top - before;
      }
      if (mode === 'newer' && inserted && !pinned) newest.hidden = false;
      controls();
    }
    async function initialize() {
      if (initializing || disposed || document.hidden) return;
      const epoch = lifecycle;
      initializing = true; controls(); status.textContent = '正在加载消息流…'; status.className = 'text-secondary small';
      try {
        const data = await request('initial', 'clients/detail?' + new URLSearchParams({ client_id: item.client_id })); if (!data) return;
        receive(data); initialized = true; setPeer(data.peer_id); renderContacts(data.conversations, true); mergeMessages(data.messages, 'initial'); renderHeading();
        status.textContent = messageRows.size ? '向上翻阅历史消息' : '暂无私信记录';
      } catch (error) { status.textContent = error.message; status.className = 'text-danger small'; messages.replaceChildren(el('p', '加载失败，可点击“刷新消息”重试', 'rt-chat-placeholder text-secondary')); }
      finally { if (epoch === lifecycle) { initializing = false; controls(); } }
    }
    async function loadMessages(mode) {
      if (disposed || document.hidden || chatBusy || mode === 'older' && !hasOlder) return;
      const version = selection, selected = peer; chatBusy = true; controls(); status.className = 'text-secondary small';
      status.textContent = mode === 'older' ? '正在加载历史消息…' : '正在刷新消息…';
      try {
        for (let batch = 0; batch < 5; batch++) {
          const query = new URLSearchParams({ client_id: item.client_id, peer_id: selected });
          if (mode === 'older') query.set('cursor', nextCursor);
          if (mode === 'newer') { if (selected) query.set('after_seq', latestSeq); else query.set('after', latestCursor); }
          const data = await request('chat', 'clients/messages?' + query); if (!data || version !== selection) return;
          receive(data); mergeMessages(data.messages, mode); renderHeading();
          status.textContent = '已更新 · ' + AdminTime.time(new Date().toISOString());
          if (mode !== 'newer' || !data.messages.has_more) break;
          if (batch === 4) status.textContent = '还有新消息，继续刷新可加载';
        }
      } catch (error) { if (version === selection) { status.textContent = error.message; status.className = 'text-danger small'; } }
      finally { if (version === selection) { chatBusy = false; controls(); } }
    }
    async function selectPeer(id) {
      if (peer === id || initializing) return;
      active.get('chat')?.controller.abort(); chatBusy = false; setPeer(id); await loadMessages('initial');
    }
    async function loadContacts(append) {
      if (disposed || document.hidden || contactsBusy || append && !hasContacts) return;
      const epoch = lifecycle;
      contactsBusy = true; controls();
      contactsError.textContent = '';
      try {
        const query = new URLSearchParams({ client_id: item.client_id }); if (append) query.set('cursor', nextContacts);
        const data = await request('contacts', 'clients/conversations?' + query); if (!data) return;
        receive(data); renderContacts(data.conversations, append); renderHeading();
      } catch (error) { contactsError.textContent = error.message; }
      finally { if (epoch === lifecycle) { contactsBusy = false; controls(); } }
    }
    function refreshAll() {
      if (disposed || document.hidden) return;
      if (!initialized) { initialize(); return; }
      loadContacts(false); loadMessages(chatReady ? 'newer' : 'initial');
    }
    let lastTop = 0, touchY = null;
    feed.addEventListener('scroll', () => {
      const top = feed.scrollTop; if (top < lastTop && top < 50) loadMessages('older'); lastTop = top;
      if (feed.scrollHeight - top - feed.clientHeight < 64) newest.hidden = true;
    }, { passive: true });
    feed.addEventListener('wheel', event => { if (event.deltaY < 0 && feed.scrollTop < 50) loadMessages('older'); }, { passive: true });
    feed.addEventListener('touchstart', event => { touchY = feed.scrollTop < 30 ? event.touches[0]?.clientY : null; }, { passive: true });
    feed.addEventListener('touchend', event => { if (touchY != null && event.changedTouches[0]?.clientY - touchY > 50) loadMessages('older'); touchY = null; }, { passive: true });
    allContacts.addEventListener('click', () => selectPeer(''));
    older.addEventListener('click', () => loadMessages('older')); moreContacts.addEventListener('click', () => loadContacts(true));
    newest.addEventListener('click', bottom); refresh.addEventListener('click', refreshAll);
    renderHeading(); initialize();
    return { refresh: refreshAll, stop };
  }
  window.RealtimeChat = { mount };
}());
