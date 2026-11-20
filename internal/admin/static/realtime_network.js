(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  function el(tag, text, className) {
    const element = document.createElement(tag);
    if (text != null) element.textContent = String(text);
    if (className) element.className = className;
    return element;
  }

  function mount({ api, showConnection }) {
    let mode = 'messages', controller, generation = 0, loading = false, active = null, current = null, returnFocus = null;
    const shell = $('networkShell'), pane = $('networkDetail'), sheet = $('networkSheet');
    const paneTitle = $('networkDetailTitle'), paneBody = $('networkDetailBody'), closeButton = $('networkClose');
    const queryInput = $('networkQuery'), searchStatus = $('networkSearchStatus'), dragButton = $('networkDrag');
    const narrow = matchMedia('(max-width: 1100px)');
    const chart = RealtimeGraph.create($('networkCanvas'), { onSelect: select, onLost: selectionLost });

    function syncDetailPlacement() {
      if (pane.hidden) return;
      shell.classList.toggle('has-detail', !narrow.matches);
      if (narrow.matches) {
        if (pane.parentNode !== sheet) sheet.append(pane);
        if (!sheet.open) sheet.showModal();
      } else {
        if (sheet.open) sheet.close();
        if (pane.parentNode !== shell) shell.append(pane);
      }
    }

    function close(restoreFocus = false) {
      active?.dispose?.(); active = null;
      paneBody.replaceChildren(); pane.hidden = true;
      if (sheet.open) sheet.close();
      shell.classList.remove('has-detail');
      chart.clearSelection();
      if (restoreFocus) (returnFocus?.isConnected ? returnFocus : $(mode + 'Network')).focus({ preventScroll: true });
      returnFocus = null;
    }

    function open(title, trigger) {
      active?.dispose?.(); active = null;
      const focusWasInPane = pane.contains(document.activeElement);
      if (pane.hidden || !narrow.matches) returnFocus = trigger;
      paneTitle.textContent = title; paneBody.replaceChildren(); pane.hidden = false;
      syncDetailPlacement();
      if (focusWasInPane) closeButton.focus({ preventScroll: true });
    }

    function client(node) {
      return { client_id: node.client_id, user_id: node.user_id, nickname: node.label, module: mode, session_id: node.session_id, connected: true };
    }

    function renderNode(state, user, data) {
      const byID = new Map(data.nodes.map(node => [node.id, node]));
      const relations = data.edges.filter(edge => edge.source === user.id || edge.target === user.id)
        .map(edge => ({ edge, peer: byID.get(edge.source === user.id ? edge.target : edge.source) }))
        .filter(relation => relation.peer);
      const key = JSON.stringify([user, relations]);
      if (key === state.key) return;
      state.key = key;
      paneTitle.textContent = user.label;
      const group = el('section', null, 'rt-network-related');
      group.append(el('h4', '图中的关联用户' + (relations.length ? ' · ' + relations.length : '')));
      if (!relations.length) group.append(el('p', '当前图中没有与此用户相连的私信关系。', 'text-secondary small'));
      for (const { edge, peer } of relations) {
        const button = el('button', null, 'btn rt-chat-contact');
        button.type = 'button'; button.dataset.edgeId = edge.id;
        const text = el('span', null, 'rt-chat-contact-text');
        text.append(el('strong', peer.label), el('small', '用户 ' + peer.user_id));
        button.append(RealtimeChat.avatar({ id: peer.user_id, nickname: peer.label, avatar_url: peer.avatar_url }), text);
        button.addEventListener('click', () => chart.selectEdge(edge.id));
        group.append(button);
      }
      const info = el('dl', null, 'realtime-detail-meta rt-network-node-meta');
      info.append(el('dt', '用户 ID'), el('dd', user.user_id), el('dt', '在线连接'), el('dd', user.connections));
      const focused = paneBody.contains(document.activeElement) ? document.activeElement : null;
      paneBody.replaceChildren(group, info);
      if (user.client_id) {
        const details = el('button', '查看连接详情', 'btn btn-sm btn-outline-secondary');
        details.type = 'button'; details.dataset.connectionDetail = '';
        details.addEventListener('click', () => {
          const item = client(user);
          if (sheet.open) close(true);
          showConnection(item);
        });
        paneBody.append(details);
      }
      if (focused) {
        const match = Array.from(paneBody.querySelectorAll('button')).find(button => focused.dataset.edgeId
          ? button.dataset.edgeId === focused.dataset.edgeId : button.hasAttribute('data-connection-detail'));
        (match || closeButton).focus({ preventScroll: true });
      }
    }

    function select({ value, data, trigger }) {
      if (value.kind === 'direct') {
        const a = data.nodes.find(node => node.id === value.source), b = data.nodes.find(node => node.id === value.target);
        if (!a || !b || active?.type === 'direct' && active.edgeID === value.id) return;
        open(a.label + ' ↔ ' + b.label, trigger);
        const lifetime = new AbortController(), host = el('section', null, 'rt-chat-panel'), item = client(a);
        paneBody.append(host);
        const chat = RealtimeChat.mount({ host, sidebar: el('div'), item, signal: lifetime.signal, api, initialPeer: b.user_id, locked: true, onMetadata() {} });
        active = { type: 'direct', edgeID: value.id, item, stop: () => chat.stop(), refresh: () => chat.refresh(), dispose: () => lifetime.abort() };
      } else if (mode === 'companions') {
        const { session_id: sessionID } = value, userID = value.kind === 'user' ? value.user_id : '';
        if (active?.type === 'room' && active.sessionID === sessionID && active.userID === userID) return;
        open(value.label + (userID ? ' · 个人事件' : ' · 群聊与事件'), trigger);
        active = RealtimeRoom.mount({ host: paneBody, sessionID, userID, api, onTitle(title) { paneTitle.textContent = title; } });
      } else {
        if (active?.type !== 'node' || active.id !== value.id) {
          open(value.label, trigger);
          active = { type: 'node', id: value.id };
        }
        renderNode(active, value, data);
      }
    }

    function selectionLost(previous, snapshot) {
      const focusWasInPane = paneBody.contains(document.activeElement);
      active?.dispose?.(); active = null;
      let message = '本次采样未展示所选关系或节点。';
      if (snapshot.sampled_at && snapshot.status === 'ok' && snapshot.complete) {
        if (previous.kind === 'edge' && previous.value) {
          const { source, target } = previous.value;
          const present = snapshot.nodes.filter(node => node.id === source || node.id === target).length;
          message = ['双方已下线。', '一方已下线。', '所选私信关系已不在当前图中。'][present];
        } else if (previous.kind === 'room') message = '当前房间没有在线成员。';
        else message = mode === 'messages' ? '该用户已下线。' : '该成员已从房间的在线列表移除。';
      }
      const notice = el('p', message, 'mb-0'); notice.setAttribute('role', 'status');
      paneBody.replaceChildren(notice);
      if (focusWasInPane || document.activeElement === document.body && returnFocus && !returnFocus.isConnected) closeButton.focus({ preventScroll: true });
    }

    function setSurface(hasNodes, emptyText) {
      $('networkStage').classList.toggle('is-empty', !hasNodes);
      $('networkCanvas').setAttribute('aria-hidden', String(!hasNodes));
      for (const id of ['networkSearch', 'networkZoom', 'networkDrag', 'networkNote']) $(id).hidden = !hasNodes;
      $('networkEmpty').hidden = hasNodes;
      $('networkEmpty').textContent = hasNodes ? '' : emptyText;
      if (!hasNodes) {
        dragButton.setAttribute('aria-pressed', 'false');
        chart.setTouchDrag(false);
      }
    }

    function renderSnapshot(data) {
      const users = data.nodes.filter(node => node.kind === 'user');
      const rooms = data.nodes.filter(node => node.kind === 'room');
      const known = Boolean(data.sampled_at);
      const stale = data.status === 'stale';
      const partial = data.status === 'partial';
      if (!known) $('networkCount').textContent = '在线状态未知';
      else {
        const counts = mode === 'messages' ? data.total_users + ' 位用户' : data.total_rooms + ' 个房间 · ' + data.total_users + ' 位用户';
        const prefix = stale ? '上次采样 ' : partial ? '已采样 ' : '在线 ';
        const showUsersCount = data.limited || data.total_users !== users.length;
        const showRoomsCount = showUsersCount || data.total_rooms !== rooms.length;
        const shown = mode === 'messages'
          ? (showUsersCount ? ' · 图中 ' + users.length + ' 位用户' : '') + ' · ' + data.edges.length + ' 条私信关系'
          : (showRoomsCount ? ' · 图中 ' + rooms.length + ' 个房间 / ' + users.length + ' 位成员' : '');
        $('networkCount').textContent = prefix + counts + shown;
      }
      $('networkSample').textContent = known ? (stale ? '上次采样 ' : '采样 ') + AdminTime.dateTime(data.sampled_at) : '暂无采样';
      $('networkStatus').textContent = [
        data.message,
        data.limited ? '图中仅展示部分节点或关系' : '',
        partial && !data.message ? '连接只采样到一部分' : ''
      ].filter(Boolean).join(' · ');
      $('networkNote').textContent = mode === 'messages'
        ? '点连线看私信，点头像看关联用户。'
        : '点房间圈看群聊与关键事件，点头像看成员记录。';
      const domain = mode === 'messages' ? '消息提醒用户' : '同行成员';
      const empty = !known ? '连接数据尚未就绪'
        : stale ? '上次采样没有展示在线' + domain
        : partial || data.limited ? '本次采样未展示在线' + domain
        : '当前没有在线的' + domain;
      setSurface(users.length > 0, empty);
    }

    async function refresh(force = false) {
      if (document.hidden || loading && !force) return;
      controller?.abort(); const task = new AbortController(), version = ++generation; controller = task; loading = true;
      let timedOut = false; const timeout = setTimeout(() => { timedOut = true; task.abort(); }, 10000);
      try {
        const data = await api('network?' + new URLSearchParams({ module: mode }), task.signal);
        if (version !== generation || task.signal.aborted) return;
        current = data;
        $('networkError').textContent = '';
        renderSnapshot(data);
        chart.setData(data);
        const selected = chart.selection()?.value;
        if (active?.type === 'direct' && selected) {
          const source = data.nodes.find(node => node.id === selected.source);
          if (source) active.item.client_id = source.client_id;
        }
        if (active?.type === 'node' && selected) renderNode(active, selected, data);
        active?.refresh?.();
      } catch (error) {
        if (version === generation && (timedOut || !task.signal.aborted)) {
          $('networkError').textContent = (timedOut ? '网络查询超时' : error.message) + (current ? '，保留上次关系图。' : '');
          if (current?.sampled_at) renderSnapshot({ ...current, status: 'stale' });
          else {
            $('networkSample').textContent = '暂无采样';
            $('networkCount').textContent = '在线状态未知';
            setSurface(false, '关系图暂不可用，可刷新重试');
          }
        }
      } finally { clearTimeout(timeout); if (version === generation) loading = false; }
    }

    for (const next of ['messages', 'companions']) $(next + 'Network').addEventListener('click', () => {
      if (mode === next) return;
      mode = next; close(); current = null;
      setSurface(false, '正在加载关系图…');
      chart.setData({ nodes: [], edges: [], module: mode });
      queryInput.value = '';
      queryInput.placeholder = mode === 'messages' ? '昵称 / 用户 ID' : '昵称 / 用户 ID / 房间 ID';
      searchStatus.textContent = '';
      for (const id of ['networkCount', 'networkStatus', 'networkError']) $(id).textContent = '';
      $('networkSample').textContent = '等待采样';
      for (const name of ['messages', 'companions']) {
        $(name + 'Network').classList.toggle('active', name === mode);
        $(name + 'Network').setAttribute('aria-pressed', String(name === mode));
      }
      refresh(true);
    });
    $('networkSearch').addEventListener('submit', event => {
      event.preventDefault();
      const query = queryInput.value.trim();
      searchStatus.textContent = query && !chart.find(query) ? '当前图中未找到匹配的用户或房间' : '';
    });
    queryInput.addEventListener('input', () => { searchStatus.textContent = ''; });
    $('networkZoomIn').addEventListener('click', () => chart.zoom(1.2));
    $('networkZoomOut').addEventListener('click', () => chart.zoom(1 / 1.2));
    $('networkFit').addEventListener('click', () => chart.fit());
    dragButton.addEventListener('click', () => {
      const enabled = dragButton.getAttribute('aria-pressed') !== 'true';
      dragButton.setAttribute('aria-pressed', String(enabled));
      chart.setTouchDrag(enabled);
    });
    closeButton.addEventListener('click', () => close(true));
    sheet.addEventListener('cancel', event => { event.preventDefault(); close(true); });
    sheet.addEventListener('click', event => { if (event.target === sheet) close(true); });
    narrow.addEventListener('change', syncDetailPlacement);
    return { refresh, stop() { controller?.abort(); generation++; loading = false; active?.stop?.(); chart.pause(); } };
  }
  window.RealtimeNetwork = { mount };
}());
