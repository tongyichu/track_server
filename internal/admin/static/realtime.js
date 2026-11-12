(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const date = value => AdminTime.dateTime(value);
  const moduleName = value => ({ messages: '消息提醒', companions: '与友同行', server: '服务端', unknown: '未知' }[value] || '未知');
  const kindName = value => ({ auth: '鉴权拒绝', acl: '权限拒绝', publish: '发布失败', connection: '发布连接异常' }[value] || value);
  const lists = Object.fromEntries(['clients', 'events'].map(name => [name, { page: 1, hasMore: false, filters: {}, generation: 0, loading: false, rendered: null }]));
  let summaryController, interval, closeDetail, refreshDetail;
  async function api(path, signal) {
    const response = await fetch('/admin/api/realtime/' + path, { credentials: 'same-origin', signal, cache: 'no-store' });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || '实时通信数据暂不可用');
    return body.data;
  }
  function node(tag, text, className) {
    const element = document.createElement(tag);
    if (text != null) element.textContent = String(text);
    if (className) element.className = className;
    return element;
  }
  function cell(row, value, secondary) {
    const td = node('td', value == null || value === '' ? '—' : value);
    if (secondary) td.append(node('small', secondary, 'text-secondary'));
    row.append(td);
    return td;
  }
  function emptyRow(host, columns, text) {
    const row = node('tr'), td = node('td', text, 'text-secondary');
    td.colSpan = columns; row.append(td); host.replaceChildren(row);
  }
  async function summary() {
    summaryController?.abort(); const controller = new AbortController(); summaryController = controller;
    let timedOut = false; const timeout = setTimeout(() => { timedOut = true; controller.abort(); }, 10000);
    try {
      const data = await api('summary', controller.signal); if (controller !== summaryController) return;
      const r = data.runtime, b = data.backlog;
      $('publisherState').textContent = !r.publisher_configured ? '未启用' : r.publisher_connected ? '已连接' : '已断开';
      $('publisherState').className = r.publisher_connected ? 'text-success' : 'text-secondary';
      $('publisherDetail').textContent = '最近连接：' + date(r.last_connected_at);
      $('pendingCount').textContent = b.status === 'ok' ? b.pending : '—';
      $('pendingDetail').textContent = b.status === 'ok' ? '待修复发送日志：' + b.journals + ' 条' : '消息持久化统计暂不可用';
      $('publishFailures').textContent = r.publish_failure;
      $('publishDetail').textContent = '发布成功：' + r.publish_success + ' 次 · 最近耗时 ' + r.last_latency_ms + ' ms';
      $('heartbeatAt').textContent = '最近心跳：' + date(r.last_heartbeat_at);
      $('publishedAt').textContent = '最近发布成功：' + date(r.last_success_at);
      $('runtimeSince').textContent = '统计起点：' + date(r.started_at);
      $('authFailures').textContent = '鉴权拒绝：' + r.auth_denied;
      $('aclFailures').textContent = '权限拒绝：' + r.acl_denied;
      $('connectionLoss').textContent = '连接异常：' + r.connection_failure;
      $('oldestPending').textContent = '最早待处理记录：' + date(b.oldest_record_at);
      $('summaryError').textContent = '';
    } catch (error) {
      if ((timedOut || !controller.signal.aborted) && controller === summaryController) $('summaryError').textContent = (timedOut ? '统计查询超时' : error.message) + '，保留上次统计。';
    } finally { clearTimeout(timeout); }
  }
  function renderClients(data) {
    const known = !!data.sampled_at, stale = data.status === 'stale';
    $('onlineCount').textContent = known ? (data.complete ? '' : '至少 ') + data.online : '—';
    $('onlineCount').className = stale ? 'text-warning' : '';
    $('onlineDetail').textContent = known ? (stale ? '上次采样 · ' : '') + '消息 ' + data.messages_online + ' / 同行 ' + data.companions_online : '实际连接数未知';
    $('brokerStatus').textContent = (data.message || 'EMQX 正常') + (known ? ' · ' + date(data.sampled_at) : '');
    // Sampling timestamps update the detail source without replacing unchanged list rows.
    lists.clients.sample = { at: data.sampled_at, status: data.status };
    const rowsKey = JSON.stringify([known, data.items]);
    if (rowsKey === lists.clients.rendered) return;
    const fragment = document.createDocumentFragment();
    for (const item of data.items || []) {
      const row = node('tr'); cell(row, item.nickname, item.user_id); cell(row, moduleName(item.module)); cell(row, item.client_id, item.session_id);
      cell(row, item.connected ? '已连接' : '已断开'); cell(row, date(item.connected_at));
      const action = cell(row, ''), button = node('button', '详情', 'btn btn-sm btn-outline-secondary');
      action.replaceChildren(button); button.type = 'button'; button.addEventListener('click', () => showDetail(item)); fragment.append(row);
    }
    if (!data.items?.length) emptyRow($('clientRows'), 6, known ? '当前筛选没有连接记录' : '实际连接数据尚不可用');
    else $('clientRows').replaceChildren(fragment);
    lists.clients.rendered = rowsKey;
  }
  function renderEvents(data) {
    const key = JSON.stringify(data.items);
    if (key === lists.events.rendered) return;
    const fragment = document.createDocumentFragment();
    for (const item of data.items || []) {
      const row = node('tr'); cell(row, date(item.at)); cell(row, moduleName(item.module)); cell(row, kindName(item.kind)); cell(row, item.user_id, item.client_id || item.session_id); cell(row, item.reason); fragment.append(row);
    }
    if (!data.items?.length) emptyRow($('eventRows'), 5, '当前筛选没有已记录的异常'); else $('eventRows').replaceChildren(fragment);
    lists.events.rendered = key;
  }
  function pagination(name) {
    const state = lists[name];
    $(name + 'Prev').disabled = state.loading || state.page <= 1;
    $(name + 'Next').disabled = state.loading || !state.hasMore;
  }
  async function load(name, request) {
    const state = lists[name];
    if (state.loading && !request) return;
    state.controller?.abort(); const controller = new AbortController(); state.controller = controller;
    const version = ++state.generation, target = request || { page: state.page, filters: state.filters };
    let timedOut = false; const timeout = setTimeout(() => { timedOut = true; controller.abort(); }, 10000);
    state.loading = true; pagination(name); $(name + 'Error').textContent = '';
    try {
      const query = new URLSearchParams({ ...target.filters, page: String(target.page) });
      const data = await api(name + '?' + query, controller.signal); if (version !== state.generation || controller.signal.aborted) return;
      if (name === 'clients') renderClients(data); else renderEvents(data);
      state.page = data.page; state.hasMore = data.has_more; state.filters = target.filters;
      $(name + 'Page').textContent = data.total == null ? '连接数量未知' : '第 ' + data.page + ' 页 · ' + (name === 'clients' && !data.complete ? '已采样 ' : '') + data.total + ' 条';
    } catch (error) {
      if ((timedOut || !controller.signal.aborted) && version === state.generation) {
        $(name + 'Error').textContent = (timedOut ? '列表查询超时' : error.message) + (state.rendered === null ? '' : '，保留上次列表与分页。');
        if (state.rendered === null) emptyRow($(name === 'clients' ? 'clientRows' : 'eventRows'), name === 'clients' ? 6 : 5, '查询失败，请重试');
      }
    } finally {
      clearTimeout(timeout);
      if (version === state.generation) { state.loading = false; pagination(name); }
    }
  }
  function showDetail(item) {
    AdminUI.runOnce('realtime-client-detail', () => {
      const content = node('div'), metadata = node('dl', null, 'realtime-detail-meta');
      const sample = lists.clients.sample || {};
      function renderMetadata(client, at, status) {
        metadata.replaceChildren();
        const values = [['用户', item.nickname || item.user_id], ['用户ID', client.user_id], ['业务', moduleName(client.module)], ['客户端', client.client_id], ['房间', client.session_id || '—'], ['状态', client.connected ? '已连接' : '已断开'], ['连接时间', date(client.connected_at)], ['断开时间', date(client.disconnected_at)], ['订阅数', client.subscriptions], ['排队消息', client.queued], ['采样时间', date(at) + (status === 'stale' ? '（上次采样）' : '')]];
        for (const [label, value] of values) metadata.append(node('dt', label), node('dd', value));
      }
      renderMetadata(item, sample.at, sample.status);
      const direct = item.module === 'messages';
      content.append(metadata, node('h3', direct ? '最近私信' : '最近房间弹幕', 'realtime-detail-title'));
      content.append(node('p', direct ? '该用户全部设备最近30条私信，含发送与接收。正文来自业务存储；MQTT 仅发送刷新提醒，记录不代表此连接已收取或已读。' : '所在房间最近30条弹幕，含其他成员发送的内容。记录来自业务存储，不代表此连接已收取；定位和控制事件不在此列表中。', 'text-secondary small'));
      const toolbar = node('div', null, 'realtime-heading mb-2'), status = node('span', '正在加载内容…', 'text-secondary small'), refresh = node('button', '刷新消息', 'btn btn-sm btn-outline-secondary');
      refresh.type = 'button'; toolbar.append(status, refresh);
      const messages = node('div', null, 'realtime-messages'); content.append(toolbar, messages);
      let controller, generation = 0, rendered = null;
      const stop = () => { controller?.abort(); generation++; refresh.disabled = false; };
      return AdminUI.form({ title: '连接详情', wide: true, readOnly: true, waitForHidden: true, cancelText: '关闭', beforeFields: content, fields: [],
        onReady({ signal }) {
          closeDetail = stop;
          signal.addEventListener('abort', stop, { once: true });
          async function loadMessages() {
            if (signal.aborted || document.hidden || refresh.disabled) return;
            controller?.abort(); controller = new AbortController(); const current = controller, version = ++generation;
            let timedOut = false; const timeout = setTimeout(() => { timedOut = true; current.abort(); }, 10000);
            refresh.disabled = true; status.textContent = '正在加载内容…'; status.className = 'text-secondary small';
            try {
              const data = await api('clients/detail?' + new URLSearchParams({ client_id: item.client_id }), current.signal);
              if (signal.aborted || version !== generation || current.signal.aborted) return;
              renderMetadata(data.client, data.sampled_at, data.status);
              const key = JSON.stringify(data.items);
              if (key !== rendered) {
                const fragment = document.createDocumentFragment();
                for (const message of data.items || []) {
                  const article = node('article', null, 'realtime-message');
                  const direction = direct ? (message.sender_id === item.user_id ? '发送给 ' + message.receiver_id : '收到来自 ' + message.sender_id) : '用户 ' + message.sender_id;
                  article.append(node('div', direction + ' · ' + date(message.created_at), 'text-secondary small'), node('p', message.content || (message.track_id ? '分享轨迹' : '（无文本内容）')));
                  if (message.track_id) article.append(node('small', '轨迹：' + message.track_id, 'text-secondary'));
                  fragment.append(article);
                }
                if (!data.items?.length) fragment.append(node('p', direct ? '暂无私信记录' : '暂无房间弹幕', 'text-secondary'));
                messages.replaceChildren(fragment); rendered = key;
              }
              status.textContent = '最近 ' + data.items.length + ' 条 · ' + date(new Date().toISOString()) + ' 更新';
            } catch (error) {
              if (signal.aborted || version !== generation || (!timedOut && current.signal.aborted)) return;
              status.textContent = (timedOut ? '内容查询超时' : error.message) + (rendered === null ? '' : '，保留上次内容'); status.className = 'text-danger small';
            } finally { clearTimeout(timeout); if (version === generation) refresh.disabled = false; }
          }
          refresh.addEventListener('click', loadMessages);
          refreshDetail = loadMessages;
          loadMessages();
        },
        onClosed() { stop(); if (closeDetail === stop) { closeDetail = null; refreshDetail = null; } }
      });
    });
  }
  for (const name of ['clients', 'events']) {
    $(name + 'Filters').addEventListener('submit', event => {
      event.preventDefault(); const filters = { module: $(name + 'Module').value, q: $(name + 'Query').value.trim() };
      if (name === 'clients') filters.state = $('clientsState').value;
      load(name, { page: 1, filters });
    });
    $(name + 'Prev').addEventListener('click', () => { const state = lists[name]; if (!state.loading && state.page > 1) load(name, { page: state.page - 1, filters: state.filters }); });
    $(name + 'Next').addEventListener('click', () => { const state = lists[name]; if (!state.loading && state.hasMore) load(name, { page: state.page + 1, filters: state.filters }); });
  }
  function refreshAll() { summary(); load('clients'); load('events'); refreshDetail?.(); }
  $('realtimeRefresh').addEventListener('click', refreshAll);
  function schedule() { clearInterval(interval); if ($('realtimeAuto').checked && !document.hidden) interval = setInterval(refreshAll, 15000); }
  function stopRequests() { summaryController?.abort(); for (const state of Object.values(lists)) state.controller?.abort(); closeDetail?.(); }
  $('realtimeAuto').addEventListener('change', schedule);
  document.addEventListener('visibilitychange', () => { if (document.hidden) stopRequests(); else refreshAll(); schedule(); });
  window.addEventListener('pagehide', () => { clearInterval(interval); stopRequests(); });
  window.addEventListener('pageshow', schedule);
  refreshAll(); schedule();
}());
