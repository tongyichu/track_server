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
        const room = n.kind === 'membership' ? byID.get(n.source) : n;
        if (active?.type === 'room' && active.sessionID === room.session_id) return;
        open(room.kind === 'room' ? room.label : '同行房间'); showRoom(room.session_id);
      } else {
        open(n.label);
        const info = el('dl', null, 'realtime-detail-meta rt-network-node-meta');
        for (const [label, value] of [['用户', n.label], ['用户ID', n.user_id], ['在线连接', n.connections], ['采样时间', AdminTime.dateTime(data.sampled_at)]]) info.append(el('dt', label), el('dd', value));
        paneBody.append(info, el('p', '点击相连的线，查看双方私信往来。', 'text-secondary small'), button('查看连接详情', () => showConnection(client(n))));
      }
    }
    function showRoom(sessionID) {
      const status = el('p', '正在加载房间…', 'text-secondary small'), content = el('div'), refresh = button('刷新房间', () => detail.refresh());
      paneBody.append(status, refresh, content);
      let request, version = 0, busy = false, disposed = false, rendered = null;
      const detail = {
        type: 'room', sessionID,
        stop() { request?.abort(); version++; busy = false; refresh.disabled = false; },
        dispose() { disposed = true; this.stop(); },
        async refresh() {
          if (busy || disposed || document.hidden) return;
          busy = true; refresh.disabled = true; const currentVersion = ++version, task = new AbortController(); request = task;
          let timedOut = false; const timeout = setTimeout(() => { timedOut = true; task.abort(); }, 10000);
          try {
            const data = await api('network/room?' + new URLSearchParams({ session_id: sessionID }), task.signal);
            if (disposed || task.signal.aborted || currentVersion !== version) return;
            paneTitle.textContent = data.room.title || '同行房间';
            const fingerprint = JSON.stringify([data.room, data.members, data.positions, data.messages, data.people]);
            if (fingerprint !== rendered) {
              const fragment = document.createDocumentFragment();
              fragment.append(el('p', data.room.session_id, 'text-secondary small'), el('h3', '成员与位置快照', 'realtime-detail-title'));
              for (const member of data.members) {
                const person = data.people[member.user_id], position = data.positions[member.user_id], row = el('article', null, 'rt-network-member');
                row.append(el('strong', (person?.nickname || member.user_id) + (member.role === 'owner' ? ' · 房主' : '')), el('small', member.user_id, 'text-secondary'));
                const state = { joined: '已加入', left: '已离开', kicked: '已移出', ended: '已结束' }[member.member_status] || member.member_status;
                row.append(el('small', state + ' · ' + (member.connections ? member.connections + ' 条在线连接' : '未采样到在线连接'), 'text-secondary'));
                if (position) row.append(el('small', position.coordinate_system + ' · ' + Number(position.latitude).toFixed(5) + ', ' + Number(position.longitude).toFixed(5) + ' · ' + AdminTime.dateTime(position.recorded_at), 'text-secondary'));
                else row.append(el('small', '暂无位置快照', 'text-secondary'));
                fragment.append(row);
              }
              fragment.append(el('h3', '最近房间弹幕', 'realtime-detail-title'));
              for (const message of data.messages) {
                const row = el('article', null, 'rt-network-member'); row.append(el('small', (data.people[message.sender_id]?.nickname || message.sender_id) + ' · ' + AdminTime.dateTime(message.created_at), 'text-secondary'), el('p', message.content)); fragment.append(row);
              }
              if (!data.messages.length) fragment.append(el('p', '暂无弹幕', 'text-secondary small'));
              content.replaceChildren(fragment); rendered = fingerprint;
            }
            status.className = 'text-secondary small'; status.textContent = '连接采样：' + AdminTime.dateTime(data.sampled_at) + (data.status === 'stale' ? '（上次采样）' : '');
          } catch (error) {
            if (!disposed && currentVersion === version && (timedOut || !task.signal.aborted)) { status.className = 'text-danger small'; status.textContent = (timedOut ? '房间查询超时' : error.message) + (rendered ? '，保留上次内容' : ''); }
          } finally { clearTimeout(timeout); if (currentVersion === version) { busy = false; refresh.disabled = false; } }
        }
      };
      active = detail; detail.refresh();
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
        $('networkNote').textContent = mode === 'messages' ? '节点为在线账号，线为双方已有的私信关系。点击线查看往来；最后一条连接断开后，节点和相关连线在下次成功采样后移除。' : '每个房间独立成组，连线表示连接所属房间；只绘制已采样的在线成员，不同房间之间没有连线。';
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
