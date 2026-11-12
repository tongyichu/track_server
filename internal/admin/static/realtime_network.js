(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  function el(tag, text, className) { const element = document.createElement(tag); if (text != null) element.textContent = String(text); if (className) element.className = className; return element; }
  function button(text, action) { const element = el('button', text, 'btn btn-sm btn-outline-secondary'); element.type = 'button'; element.addEventListener('click', action); return element; }
  function mount({ api, showConnection }) {
    let mode = 'messages', controller, generation = 0, loading = false, active = null, current = null;
    const shell = $('networkShell'), pane = $('networkDetail'), paneTitle = $('networkDetailTitle'), paneBody = $('networkDetailBody');
    const chart = RealtimeGraph.create($('networkCanvas'), { onSelect: select, onLost: close });
    function close() { active?.dispose(); active = null; paneBody.replaceChildren(); pane.hidden = true; shell.classList.remove('has-detail'); chart.clearSelection(); }
    function open(title) { active?.dispose(); active = null; paneTitle.textContent = title; paneBody.replaceChildren(); pane.hidden = false; shell.classList.add('has-detail'); }
    function client(node) { return { client_id: node.client_id, user_id: node.user_id, nickname: node.label, module: mode, session_id: node.session_id, connected: true }; }
    function select(selection) {
      const n = selection.value, data = selection.data, byID = new Map(data.nodes.map(n => [n.id, n]));
      if (n.kind === 'direct') {
        const a = byID.get(n.source), b = byID.get(n.target); if (!a || !b) return;
        if (active?.type === 'direct' && active.edgeID === n.id) return;
        open(a.label + ' ↔ ' + b.label);
        const lifetime = new AbortController(), host = el('section', null, 'rt-chat-panel'), sidebar = el('div'), item = client(a);
        paneBody.append(host);
        const chat = RealtimeChat.mount({ host, sidebar, item, signal: lifetime.signal, api, initialPeer: b.user_id, locked: true, onMetadata() {} });
        active = { type: 'direct', edgeID: n.id, item, stop: () => chat.stop(), refresh: () => chat.refresh(), dispose: () => lifetime.abort() };
      } else if (mode === 'companions') {
        const room = n;
        if (!room) return;
        const userID = n.kind === 'user' ? n.user_id : '';
        if (active?.type === 'room' && active.sessionID === room.session_id && active.userID === userID) return;
        open(userID ? n.label + ' · 个人事件' : room.label + ' · 群聊与事件');
        active = RealtimeRoom.mount({ host: paneBody, sessionID: room.session_id, userID, api, onTitle(title) { paneTitle.textContent = title; } });
      } else {
        open(n.label);
        const info = el('dl', null, 'realtime-detail-meta rt-network-node-meta');
        for (const [label, value] of [['用户', n.label], ['用户ID', n.user_id], ['在线连接', n.connections], ['采样时间', AdminTime.dateTime(data.sampled_at)]]) info.append(el('dt', label), el('dd', value));
        paneBody.append(info, el('p', '点击相连的线，查看双方私信往来。', 'text-secondary small'), button('查看连接详情', () => showConnection(client(n))));
      }
    }
    async function refresh(force = false) {
      if (document.hidden || loading && !force) return;
      controller?.abort(); const task = new AbortController(), version = ++generation; controller = task; loading = true;
      let timedOut = false; const timeout = setTimeout(() => { timedOut = true; task.abort(); }, 10000);
      $('networkError').textContent = '';
      try {
        const data = await api('network?' + new URLSearchParams({ module: mode }), task.signal);
        if (version !== generation || task.signal.aborted) return;
        current = data; chart.setData(data);
        const users = data.nodes.filter(n => n.kind === 'user'), rooms = data.nodes.filter(n => n.kind === 'room');
        $('networkEmpty').hidden = users.length > 0;
        $('networkEmpty').textContent = !data.sampled_at ? '连接数据尚未就绪' : mode === 'messages' ? '当前没有在线的消息提醒用户' : '当前没有在线的同行成员';
        $('networkCount').textContent = data.sampled_at ? mode === 'messages' ? users.length + ' 位用户 · ' + data.edges.length + ' 条私信关系' : rooms.length + ' 个房间 · ' + users.length + ' 个成员节点' : '在线状态未知';
        $('networkStatus').textContent = [data.message, data.sampled_at ? (data.status === 'stale' ? '上次采样：' : '采样：') + AdminTime.dateTime(data.sampled_at) : '', data.limited ? '局部展示：最多200个用户节点、50个房间、400条私信关系' : ''].filter(Boolean).join(' · ');
        $('networkNote').textContent = mode === 'messages' ? '节点为在线账号，线为双方已有的私信关系。点击线查看往来；最后一条连接断开后，节点和相关连线在下次成功采样后移除。' : '每个大圈是一个房间，圈内头像为在线成员，房主带专属标签。点大圈查看群聊与关键事件，点头像查看个人事件；可拖动整个房间，成员头像限在所属圈内拖动。';
        const selected = chart.selection();
        if (active?.type === 'direct' && selected?.value?.source) {
          const source = data.nodes.find(n => n.id === selected.value.source); if (source) active.item.client_id = source.client_id;
        }
        active?.refresh?.();
      } catch (error) {
        if (version === generation && (timedOut || !task.signal.aborted)) {
          $('networkError').textContent = (timedOut ? '网络查询超时' : error.message) + (current ? '，保留上次关系图。' : '');
          if (!current) { $('networkEmpty').hidden = false; $('networkEmpty').textContent = '关系图暂不可用，可刷新重试'; }
        }
      } finally { clearTimeout(timeout); if (version === generation) loading = false; }
    }
    for (const next of ['messages', 'companions']) $(next + 'Network').addEventListener('click', () => {
      if (mode === next) return; mode = next; close(); current = null; chart.setData({ nodes: [], edges: [], module: mode });
      $('networkEmpty').hidden = false; $('networkEmpty').textContent = '正在加载关系图…'; $('networkCount').textContent = ''; $('networkStatus').textContent = '';
      for (const name of ['messages', 'companions']) { $(name + 'Network').classList.toggle('active', name === mode); $(name + 'Network').setAttribute('aria-pressed', String(name === mode)); }
      refresh(true);
    });
    $('networkSearch').addEventListener('submit', event => { event.preventDefault(); const query = $('networkQuery').value.trim(); if (query && !chart.find(query)) $('networkError').textContent = '当前已显示的节点中未找到该用户或房间'; });
    $('networkZoomIn').addEventListener('click', () => chart.zoom(1.2)); $('networkZoomOut').addEventListener('click', () => chart.zoom(1 / 1.2)); $('networkFit').addEventListener('click', () => chart.fit()); $('networkClose').addEventListener('click', close);
    return { refresh, stop() { controller?.abort(); generation++; loading = false; active?.stop(); chart.pause(); } };
  }
  window.RealtimeNetwork = { mount };
}());
