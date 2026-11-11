(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const date = value => AdminTime.dateTime(value);
  const moduleName = value => ({ messages: '消息提醒', companions: '与友同行', server: '服务端', unknown: '未知' }[value] || '未知');
  const kindName = value => ({ auth: '鉴权拒绝', acl: '权限拒绝', publish: '发布失败', connection: '发布连接异常' }[value] || value);
  let tab = 'clients', page = 1, hasMore = false, generation = 0, listController, summaryController;
  let committedPage = 1, committedMore = false, interval;
  const rendered = {};
  let filters = { module: '', q: '', state: '' };
  async function api(path, signal) {
    const response = await fetch('/admin/api/realtime/' + path, { credentials: 'same-origin', signal, cache: 'no-store' });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || '实时通信数据暂不可用');
    return body.data;
  }
  function cell(row, value, secondary) {
    const td = document.createElement('td'); td.textContent = value == null || value === '' ? '—' : String(value);
    if (secondary) { const small = document.createElement('small'); small.className = 'text-secondary'; small.textContent = secondary; td.append(small); }
    row.append(td);
  }
  function emptyRow(host, columns, text) {
    const row = document.createElement('tr'), td = document.createElement('td'); td.colSpan = columns; td.className = 'text-secondary'; td.textContent = text; row.append(td); host.replaceChildren(row);
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
      $('pendingDetail').textContent = b.status === 'ok' ? '待修复发送日志 ' + b.journals + ' 条' : '消息持久化统计暂不可用';
      $('publishFailures').textContent = r.publish_failure;
      $('publishDetail').textContent = r.publish_success + r.publish_failure ? '发布成功 ' + r.publish_success + ' 次 · 最近耗时 ' + r.last_latency_ms + ' ms' : '尚无发布记录';
      $('heartbeatAt').textContent = '最近心跳：' + date(r.last_heartbeat_at);
      $('publishedAt').textContent = '最近发布成功：' + date(r.last_success_at);
      $('runtimeSince').textContent = '统计起点：' + date(r.started_at);
      $('authFailures').textContent = '鉴权拒绝：' + r.auth_denied;
      $('aclFailures').textContent = '权限拒绝：' + r.acl_denied;
      $('connectionLoss').textContent = '连接异常：' + r.connection_failure;
      $('oldestPending').textContent = '最早待处理记录：' + date(b.oldest_record_at);
      $('summaryError').textContent = '';
    } catch (error) { if ((timedOut || !controller.signal.aborted) && controller === summaryController) $('summaryError').textContent = (timedOut ? '统计查询超时' : error.message) + '，保留上次统计。'; }
    finally { clearTimeout(timeout); }
  }
  async function load() {
    listController?.abort(); const controller = new AbortController(); listController = controller; const version = ++generation;
    let timedOut = false; const timeout = setTimeout(() => { timedOut = true; controller.abort(); }, 10000);
    $('listError').textContent = ''; $('realtimeNext').disabled = true; $('realtimePrev').disabled = true;
    const query = new URLSearchParams({ ...filters, page: String(page) });
    try {
      const data = await api(tab + '?' + query, controller.signal); if (version !== generation) return;
      const host = $(tab === 'clients' ? 'clientRows' : 'eventRows'), fragment = document.createDocumentFragment();
      if (tab === 'clients') {
        const known = !!data.sampled_at, stale = data.status === 'stale', prefix = data.complete ? '' : '至少 ';
        $('onlineCount').textContent = known ? prefix + data.online : '—';
        $('onlineCount').className = stale ? 'text-warning' : '';
        $('onlineDetail').textContent = known ? (stale ? '上次采样 · ' : '') + '消息 ' + data.messages_online + ' / 同行 ' + data.companions_online : '实际连接数未知';
        $('brokerStatus').textContent = (data.message || 'EMQX 管理接口正常') + (known ? ' · 采样于 ' + date(data.sampled_at) : '');
        for (const item of data.items || []) {
          const row = document.createElement('tr'); cell(row, item.nickname, item.user_id); cell(row, moduleName(item.module)); cell(row, item.client_id, item.session_id);
          cell(row, item.connected ? '已连接' : '已断开'); cell(row, date(item.connected_at)); cell(row, date(item.disconnected_at)); cell(row, item.subscriptions); cell(row, item.queued); fragment.append(row);
        }
        const key = JSON.stringify([known, data.items]);
        if (key !== rendered.clients) { if (!data.items?.length) emptyRow(host, 8, known ? '当前筛选没有连接记录' : '实际连接数据尚不可用'); else host.replaceChildren(fragment); rendered.clients = key; }
      } else {
        for (const item of data.items || []) { const row = document.createElement('tr'); cell(row, date(item.at)); cell(row, moduleName(item.module)); cell(row, kindName(item.kind)); cell(row, item.user_id, item.client_id); cell(row, item.reason); fragment.append(row); }
        const key = JSON.stringify(data.items);
        if (key !== rendered.events) { if (!data.items?.length) emptyRow(host, 5, '当前筛选没有已记录的异常'); else host.replaceChildren(fragment); rendered.events = key; }
      }
      hasMore = data.has_more; committedPage = page; committedMore = hasMore;
      $('realtimePage').textContent = data.total == null ? '连接数量未知' : '第 ' + data.page + ' 页 · ' + (tab === 'clients' && !data.complete ? '已采样 ' : '') + data.total + ' 条';
      $('realtimePrev').disabled = page <= 1; $('realtimeNext').disabled = !hasMore;
    } catch (error) { if ((timedOut || !controller.signal.aborted) && version === generation) { page = committedPage; hasMore = committedMore; $('realtimePrev').disabled = page <= 1; $('realtimeNext').disabled = !hasMore; $('listError').textContent = (timedOut ? '列表查询超时' : error.message) + '，保留上次列表。'; } }
    finally { clearTimeout(timeout); }
  }
  function switchTab(next) {
    tab = next; page = 1; committedPage = 1; committedMore = false;
    for (const name of ['clients', 'events']) { const selected = tab === name; $(name + 'Tab').classList.toggle('active', selected); $(name + 'Tab').setAttribute('aria-selected', String(selected)); $(name + 'Panel').hidden = !selected; }
    $('realtimeState').disabled = next !== 'clients'; load();
  }
  $('clientsTab').addEventListener('click', () => switchTab('clients')); $('eventsTab').addEventListener('click', () => switchTab('events'));
  $('realtimeFilters').addEventListener('submit', event => { event.preventDefault(); filters = { module: $('realtimeModule').value, q: $('realtimeQuery').value.trim(), state: $('realtimeState').value }; page = 1; committedPage = 1; committedMore = false; load(); });
  $('realtimePrev').addEventListener('click', () => { if (page > 1) { page--; load(); } });
  $('realtimeNext').addEventListener('click', () => { if (hasMore) { page++; load(); } });
  $('realtimeRefresh').addEventListener('click', () => { summary(); load(); });
  function schedule() { clearInterval(interval); if ($('realtimeAuto').checked && !document.hidden) interval = setInterval(() => { summary(); load(); }, 15000); }
  $('realtimeAuto').addEventListener('change', schedule);
  document.addEventListener('visibilitychange', () => { if (document.hidden) { summaryController?.abort(); listController?.abort(); } else { summary(); load(); } schedule(); });
  window.addEventListener('pagehide', () => { clearInterval(interval); summaryController?.abort(); listController?.abort(); });
  window.addEventListener('pageshow', schedule);
  summary(); load();
  schedule();
}());
