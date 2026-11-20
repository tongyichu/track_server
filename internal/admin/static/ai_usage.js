(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const featureNames = { track_auto: '自动路线说明与图案', track_description: '路线说明联网填写', track_shape: '手动轨迹图案识别', shape_learning: '人工改标学习复核', scenic_description: '景点介绍' };
  const states = { queued: '排队中', running: '执行中', cancelling: '正在中断', success: '生成成功', failed: '失败', cancelled: '已中断', interrupted: '进程中断', paused: '因暂停停止', deferred: '等待额度', not_sent: '未发出', unknown: '结果未知' };
  const stages = { waiting: '等待执行', preparing: '准备资料', search: '联网检索', generation: '模型生成', balance: '账户状态诊断', saving: '保存结果', finished: '已结束' };
  const adoptions = { pending: '待确认', applied: '已采用', cached: '已缓存待写回', not_generated: '尚未生成', not_applied: '未采用', no_result: '无可用内容', stale: '资料已变化' };
  const errorNames = { insufficient_balance: 'API 余额不足', invalid_parameters: '调用参数不兼容', authentication_failed: '凭证无效', provider_denied: '服务商拒绝', rate_limited: '调用限流', provider_unavailable: '服务商暂不可用', provider_request_failed: '供应商请求失败', network_or_response_error: '网络或响应异常', cancelled_or_timeout: '中断或超时', not_sent: '未发出请求', result_not_recorded: '执行中断，未取得最终响应记录' };
  const n = value => Number(value || 0).toLocaleString('zh-CN');
  const amount = value => Number(value || 0).toLocaleString('zh-CN', { maximumFractionDigits: 6 });
  const dateTime = value => AdminTime.dateTime(value);
  const seconds = value => (Number(value || 0) / 1000).toFixed(1) + ' 秒';
  const feature = value => featureNames[value] || value;
  const node = (tag, text, className) => { const el = document.createElement(tag); if (text != null) el.textContent = String(text); if (className) el.className = className; return el; };
  const can = (module, write) => window.AdminAccess && AdminAccess.can(module, !!write);
  let settings = null, statistics = null, cursor = '', nextCursor = '', history = [], page = 1, activeCursor = '', activeRows = [], modalTaskID = '', detailController = null;
  const filterIDs = ['feature', 'provider', 'model', 'mode', 'status', 'adoption'];
  let appliedFilters = new URLSearchParams(), renderedTasks = '', activeOpen = true, activeLoading = false;
  let policyDirty = false, pricesDirty = false, busySettings = false;
  const controllers = {}, charts = new Map(), operations = new Set();
  const modal = new AdminComponents.Dialog($('detailModal'));
  function localDate(d) { return AdminTime.date(d); }
  let quotaLoading = false, quotaSnapshot = null, tasksLoading = false, statisticsLoading = false;
  const quotaNodes = new Map(), quotaDrafts = new Map(), controlPolicies = new Map();
  async function loadQuotas(force) {
    if (quotaLoading && !force) return;
    const ctrl = replaceController('quotas'); quotaLoading = true; $('refreshQuota').disabled = true;
    try {
      const data = await request('quotas', { signal: ctrl.signal });
      if (ctrl.signal.aborted) return;
      quotaSnapshot = data; renderQuotas(data);
      if(settings)alerts(Object.assign({},settings,{policies:(data.items||[]).map(item=>({policy:item.policy,day:data.day,requests:item.used}))}));
    } catch (error) {
      if (error.name !== 'AbortError') {
        $('quotaHint').textContent = '额度读取失败，请刷新重试' + (quotaSnapshot ? '；保留上次数据' : '');
        $('settingsHint').textContent = '额度读取失败'; $('settingsHint').classList.add('text-danger');
      }
    } finally {
      if (controllers.quotas === ctrl) { quotaLoading = false; $('refreshQuota').disabled = false; }
    }
  }
  function quotaRow(id) {
    if (quotaNodes.has(id)) return quotaNodes.get(id);
    if (!quotaNodes.size) $('quotaRows').replaceChildren();
    const row = node('tr'), title = node('strong', feature(id)), meta = node('small', '', 'ai-quota-meta');
    const name = node('td'); name.append(title, meta);
    const used = node('td'), limit = node('td'), caption = node('small', '', 'ai-quota-meta'), value = node('span'), action = node('td');
    limit.append(value, caption);
    const edit = button('调整', () => editQuota(id), 'ai_settings'); action.append(edit);
    row.append(name, used, limit, action); $('quotaRows').append(row);
    const item = { row, title, meta, used, value, caption, edit };
    quotaNodes.set(id, item); return item;
  }
  function renderQuotas(data) {
    const items = data.items || [], existing = new Set(items.map(item => item.feature));
    quotaNodes.forEach((item, id) => { if (!existing.has(id)) { item.row.remove(); quotaNodes.delete(id); } });
    const exhausted = items.filter(item => !item.unlimited && item.remaining === 0).length;
    const limited = items.filter(item => !item.unlimited).length;
    $('settingsHint').textContent = !items.length ? '暂无额度信息' : exhausted ? exhausted + ' 个功能额度已用尽' : limited ? limited + ' 个功能已设每日上限' : '当前额度不限';
    $('settingsHint').classList.toggle('text-danger', exhausted > 0);
    const resetsAt = AdminTime.short(data.resets_at, true, '');
    $('quotaHint').textContent = [data.day + ' · 北京时间', resetsAt && '下次重置 ' + resetsAt].filter(Boolean).join(' · ');
    items.forEach(item => {
      const view = quotaRow(item.feature), policy = item.policy;
      view.title.textContent = feature(item.feature); view.used.textContent = n(item.used) + ' 次';
      view.value.textContent = item.unlimited ? '不限' : n(policy.daily_requests) + ' 次';
      view.caption.textContent = item.unlimited ? '' : item.remaining === 0 ? '今日已用尽' : '今日剩余 ' + n(item.remaining) + ' 次';
      view.caption.classList.toggle('text-danger', !item.unlimited && item.remaining === 0);
      view.meta.textContent = policy.pause_automatic ? '自动任务已暂停' : '';
      view.edit.disabled = !data.durable;
    });
    if (!items.length) { const row = node('tr'), cell = node('td', '暂无可用功能额度', 'hint'); cell.colSpan = 4; row.append(cell); $('quotaRows').replaceChildren(row); }
  }
  function editQuota(id) {
    if (!can('ai_settings', true)) return;
    return AdminUI.runOnce('ai-quota:' + id, async () => {
      let baseline = null;
      const values = await AdminUI.form({
        title: '调整配额 · ' + feature(id), confirmText: '保存并立即生效', waitForHidden: true,
        load: async signal => {
          const data = await request('quotas', { signal }); const item = (data.items || []).find(row => row.feature === id);
          if (!data.durable || !item) throw new Error('当前配额暂不可调整');
          baseline = item.policy;
          const intro = node('p', '今日已用 ' + n(item.used) + ' 次；当前日配额' + (item.unlimited ? '不限' : n(baseline.daily_requests) + ' 次') + '。', 'hint');
          const note = node('p', '填 0 表示不限。仅调整每日请求配额，用量和其他配置保留；低于今日已用次数时，后续请求将等待配额恢复。', 'hint');
          return { beforeFields: intro, afterFields: note, fields: [{ name: 'daily_requests', label: '每日请求配额（0＝不限）', type: 'number', value: quotaDrafts.has(id) ? quotaDrafts.get(id) : baseline.daily_requests, min: 0, step: 1, required: true }] };
        },
        onReady: view => { const input = view.form.elements.daily_requests; input.max = '100000'; input.dataset.adminInput = 'ai_settings'; const submit = view.root.querySelector('button[type="submit"]'); if (submit) submit.dataset.adminWrite = 'ai_settings'; }
      });
      if (!values || !baseline) return;
      const limit = Number(values.daily_requests); quotaDrafts.set(id, values.daily_requests);
      if (!Number.isInteger(limit) || limit < 0 || limit > 100000) { $('quotaMessage').textContent = '每日配额需为 0～100000，输入已保留。'; return; }
      const ctrl = replaceController('quota-save:' + id);
      try {
        const data = await request('policies/' + encodeURIComponent(id) + '/quota', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, signal: ctrl.signal, body: JSON.stringify({ revision: baseline.revision, daily_requests: limit }) });
        if (!data.saved || !data.policy) throw new Error('保存结果未确认，请刷新核实');
        quotaDrafts.delete(id);
        const existing = controlPolicies.get(id);
        if (existing && existing.revision === baseline.revision) Object.assign(existing, { revision: data.policy.revision, daily_requests: data.policy.daily_requests, updated_by: data.policy.updated_by, updated_at: data.policy.updated_at });
        $('quotaMessage').textContent = feature(id) + '的日配额已设为' + (limit === 0 ? '不限' : n(limit) + ' 次') + '，对后续请求立即生效，已有用量保留。';
        await loadQuotas(true); loadSettings();
      } catch (error) {
        if (error.name !== 'AbortError') $('quotaMessage').textContent = error.message + '；新配额输入已保留，重新调整时可继续使用。';
      }
    });
  }

  function setPeriod() {
    const custom = $('period').value === 'custom'; $('customDates').hidden = !custom;
    if (custom) return;
    const today = localDate(new Date()); $('from').value = AdminTime.shiftDate(today, -(Number($('period').value) || 1) + 1); $('to').value = today;
  }
  function updateFilterSummary() {
    const selected = ['provider', 'mode', 'status', 'adoption'].map(id => $(id).selectedOptions[0]).filter(option => option.value).map(option => option.textContent);
    $('filterSummary').textContent = selected.length ? '更多筛选 · ' + selected.join(' / ') : '更多筛选';
    document.querySelectorAll('[data-task-status]').forEach(button => {
      const pressed = button.dataset.taskStatus === appliedFilters.get('status');
      button.setAttribute('aria-pressed', String(pressed));
      button.classList.toggle('active', pressed);
    });
  }
  function updateSearchHint() {
    const changed = $('q').value.trim() !== (appliedFilters.get('q') || '');
    $('searchDraftHint').textContent = changed ? '按回车或点击查询应用搜索词。' : '';
  }
  function applyFilters(includeSearch = false) {
    setPeriod();
    const from = $('from').value, to = $('to').value;
    if (!from || !to || from > to || to > AdminTime.shiftDate(from, 365)) {
      $('pageError').textContent = '请选择 1～366 天的有效日期范围。';
      return;
    }
    const q = includeSearch ? $('q').value.trim() : appliedFilters.get('q');
    if (q && new TextEncoder().encode(q).length > 200) {
      $('pageError').textContent = '搜索词过长，请缩短后重试。';
      return;
    }
    const params = new URLSearchParams({ from, to });
    filterIDs.forEach(id => { if ($(id).value) params.set(id, $(id).value); });
    if (q) params.set('q', q);
    appliedFilters = params;
    updateFilterSummary();
    updateSearchHint();
    refresh(true);
  }
  function query(active) {
    const params = new URLSearchParams(appliedFilters);
    if (active) {
      ['from', 'to', 'status', 'adoption'].forEach(id => params.delete(id));
      params.set('active', 'true');
    }
    params.set('limit', '30'); return params;
  }
  async function request(path, options) {
    const response = await fetch('/admin/api/ai/' + path, Object.assign({ cache: 'no-store' }, options));
    let result; try { result = await response.json(); } catch (_) { throw new Error('服务返回异常，请稍后重试'); }
    if (!response.ok) throw new Error(result.error || '请求失败'); return result.data;
  }
  function replaceController(key) { if (controllers[key]) controllers[key].abort(); const ctrl = new AbortController(); controllers[key] = ctrl; return ctrl; }
  function showError(key, error) { if (error.name !== 'AbortError') $(key).textContent = error.message || '读取失败，请重试'; }
  function button(text, action, permission) { const el = node('button', text, 'btn btn-sm btn-outline-primary'); el.type = 'button'; if (permission) el.dataset.adminWrite = permission; el.addEventListener('click', action); return el; }
  function objectLink(t) {
    const module = t.feature === 'scenic_description' ? 'scenic_spots' : 'tracks';
    const title = (t.object_name || '') + (t.object_id ? ' · ' + t.object_id : '');
    if (!t.object_id || !can(module)) return node('span', title || '未保存的景点草稿');
    const a = node('a', title); a.href = '/admin/' + module + '.html?' + (module === 'tracks' ? 'track_id' : 'spot_id') + '=' + encodeURIComponent(t.object_id); a.target = '_blank'; a.rel = 'noopener'; return a;
  }
  function taskActions(t, target, inDetail) {
    if (!inDetail) target.append(button('详情', () => openDetail(t.id)));
    if (['queued', 'running', 'cancelling'].includes(t.status) && t.stage !== 'saving' && can('ai_tasks', true)) {
      const b = button(t.cancel_requested ? '正在中断…' : '中断', () => mutate(t, 'cancel'), 'ai_tasks'); b.disabled = !!t.cancel_requested; target.append(b);
    }
    const source = t.feature === 'scenic_description' ? 'scenic_spots' : 'tracks';
    if (t.object_id && ['failed', 'cancelled', 'interrupted'].includes(t.status) && can('ai_tasks', true) && can(source, true) && !t.retry_task_id) target.append(button('重试', () => mutate(t, 'retry'), 'ai_tasks'));
  }
  async function mutate(t, action) {
    if (operations.has(t.id)) return; operations.add(t.id);
    const confirmed = await AdminUI.confirm({ title: action === 'cancel' ? '中断 AI 任务' : '重试 AI 任务', message: action === 'cancel' ? '请求停止后续步骤和结果写回。已经发生的供应商消耗会保留，无法撤回。' : '将重新读取当前已保存的资料并调用 AI，产生新的请求和消耗。生成结果可在任务详情中查看。', confirmText: action === 'cancel' ? '确认中断' : '确认重试' });
    if (!confirmed) {operations.delete(t.id);return;}
    try { const data = await request('tasks/' + encodeURIComponent(t.id) + '/' + action, { method: 'POST' }); refresh(false); $('pageError').textContent = action === 'cancel' ? '已请求中断，正在等待执行端确认。' : '已创建重试任务：' + data.task_id; if (action === 'retry') openDetail(data.task_id); }
    catch (error) { showError('pageError', error); }
    finally { operations.delete(t.id); }
  }
  async function loadTasks(target = { cursor, history, page }) {
    const ctrl = replaceController('tasks'), params = query(false);
    tasksLoading = true;
    $('prev').disabled = $('next').disabled = true;
    if (target.cursor) params.set('cursor', target.cursor);
    try {
      const data = await request('tasks?' + params, { signal: ctrl.signal });
      if (ctrl.signal.aborted) return;
      ({ cursor, history, page } = target);
      nextCursor = data.next_cursor || '';
      renderTasks(data.items || []);
      $('total').textContent = '共 ' + n(data.total) + ' 项';
      $('pageHint').textContent = '第 ' + page + ' 页';
    } catch (error) { showError('pageError', error); }
    finally {
      if (controllers.tasks === ctrl) {
        tasksLoading = false;
        $('prev').disabled = !history.length;
        $('next').disabled = !nextCursor;
      }
    }
  }
  function renderTasks(items) {
    const key = JSON.stringify(items);
    if (key === renderedTasks) return;
    renderedTasks = key;
    const body = $('tasks'); body.replaceChildren(); if (!items.length) { const td = node('td', '当前条件下暂无任务记录', 'hint'); td.colSpan = 6; const row = node('tr'); row.append(td); body.append(row); return; }
    items.forEach(t => {
      const row = node('tr');
      const time = node('td', dateTime(t.created_at));
      const business = node('td'); business.append(node('strong', feature(t.feature)), node('br'), objectLink(t));
      const source = node('td', t.model || '—'); source.append(node('small', t.mode === 'manual' ? '手动触发' : '自动触发'));
      const status = node('td'); status.append(node('span', states[t.status] || t.status, 'ai-state ai-state-' + t.status));
      if (t.status === 'success' && t.adoption && t.adoption !== 'not_generated') status.append(node('small', adoptions[t.adoption] || t.adoption));
      if (t.error) { const error = node('small', t.error, 'ai-task-error'); error.title = t.error; status.append(error); }
      const usage = node('td', n(t.requests) + ' 次请求');
      if (t.usage_known) usage.append(node('small', n(t.input_tokens + t.output_tokens) + ' Token'));
      const actions = node('td'); taskActions(t, actions);
      row.append(time, business, source, status, usage, actions); body.append(row);
    });
  }
  function showActiveBody() {
    const hasActive = activeRows.length > 0;
    $('activeBody').hidden = !hasActive || !activeOpen;
    $('toggleActive').hidden = !hasActive;
    $('toggleActive').setAttribute('aria-expanded', String(hasActive && activeOpen));
    $('toggleActive').textContent = activeOpen ? '收起' : '展开';
  }
  async function loadActive(append) {
    if (append && (!activeCursor || activeLoading)) return;
    const ctrl = replaceController('active'), params = query(true);
    activeLoading = true; $('moreActive').disabled = true;
    if (append) params.set('cursor', activeCursor);
    try {
      const data = await request('tasks?' + params, { signal: ctrl.signal });
      if (ctrl.signal.aborted) return;
      activeCursor = data.next_cursor || '';
      activeRows = append ? activeRows.concat(data.items || []) : data.items || [];
      const hasActiveFilters = ['feature', 'provider', 'model', 'mode', 'q'].some(id => params.has(id));
      $('activeHint').textContent = n(data.total) + ' 项' + (hasActiveFilters ? ' · 实时筛选' : '');
      $('activeHint').title = hasActiveFilters ? '按功能、供应商、模型、触发方式和已查询的搜索词筛选实时任务' : '';
      $('moreActive').hidden = !activeCursor;
      const root = $('active'); root.replaceChildren();
      activeRows.forEach(t => {
        const card = node('article', null, 'ai-active-card'); card.append(node('strong', feature(t.feature)), node('p')); card.lastChild.append(objectLink(t));
        card.append(node('p', (t.mode === 'manual' ? '手动' : '自动') + ' · ' + t.model, 'hint'), node('p', (t.cancel_requested ? '正在中断' : stages[t.stage] || states[t.status] || t.status) + ' · ' + seconds(Date.now() - new Date(t.created_at).getTime())));
        const actions = node('div', null, 'ai-detail-actions'); taskActions(t, actions); card.append(actions); root.append(card);
      });
      showActiveBody();
    } catch (error) {
      if (error.name !== 'AbortError') $('activeHint').textContent = activeRows.length ? '读取失败 · 显示上次数据' : '读取失败';
    } finally {
      if (controllers.active === ctrl) { activeLoading = false; $('moreActive').disabled = false; }
    }
  }
  function metric(title, value, hint) { return AdminStats.metric({ label: title, value: value, note: hint }); }
  function coverage(known, requests) { return n(known) + ' / ' + n(requests) + ' 次'; }
  async function loadStatistics() {
    const ctrl = replaceController('statistics'), params = query(false);
    statisticsLoading = true;
    try {
      const data = await request('statistics?' + params, { signal: ctrl.signal });
      if (ctrl.signal.aborted) return;
      const from = params.get('from'), to = params.get('to');
      statistics = { data, from, to };
      let costNote = data.requests ? '可估算 ' + coverage(data.cost_known, data.requests) : '当前筛选无请求';
      if (data.requests && !data.cost_known && !data.usage_known) costNote += ' · 尚无 Token 用量记录';
      $('stats').replaceChildren(
        metric('任务数', n(data.tasks), n(data.success) + ' 成功 · ' + n(data.failed) + ' 失败或进程中断'),
        metric('请求次数', n(data.requests), '按任务创建日期归属'),
        metric('已知 Token', data.usage_known ? n(data.input_tokens + data.output_tokens) : '—', data.requests ? '有用量 ' + coverage(data.usage_known, data.requests) : ''),
        metric('费用估算', data.cost_known ? '¥' + amount(data.estimated_cny) : '—', costNote)
      );
      $('statHint').textContent = from === to ? from : from + ' 至 ' + to;
      renderBreakdown(data);
      (data.models || []).forEach(row => addModel(row.key.split(' / ').slice(1).join(' / ')));
      drawCharts(statistics);
    }
    catch (error) { if (error.name !== 'AbortError') $('statHint').textContent = statistics ? '统计读取失败 · 显示上次结果' : '统计读取失败，请点击查询重试'; }
    finally {if(controllers.statistics===ctrl)statisticsLoading=false;}
  }
  function renderBreakdown(data) {
    const root=$('breakdown');root.replaceChildren();
    for (const [key, caption] of [['features', '功能'], ['models', '模型']]) {
      for (const group of data[key] || []) {
        const row = node('tr');
        const tokens = group.usage_known ? n(group.input_tokens) + ' / ' + n(group.output_tokens) : '未知';
        const cost = group.cost_known ? '≈ ¥' + amount(group.estimated_cny) : '未知';
        const counts = group.requests ? cost + ' · ' + coverage(group.cost_known, group.requests) + '可估算' : '—';
        const values = [caption + ' · ' + (key === 'features' ? feature(group.key) : group.key),
          n(group.tasks) + ' / ' + n(group.requests), n(group.success) + ' / ' + n(group.failed), tokens, counts, n(group.adopted)];
        values.forEach(value => row.append(node('td', value)));
        root.append(row);
      }
    }
    if(!root.children.length){const row=node('tr'),cell=node('td','当前周期暂无数据','hint');cell.colSpan=6;row.append(cell);root.append(row);}
  }
  function addModel(value) { if (value && !Array.from($('model').options).some(option => option.value === value)) { const option = node('option', value); option.value = value; $('model').append(option); } }
  function daySeries(data, from, to) {
    const counts = new Map((data.days || []).map(day => [day.key, day.tasks]));
    const first = AdminTime.date(data.first_recorded_at, '');
    const rows = [];
    for (let day = from; day && day <= to; day = AdminTime.shiftDate(day, 1)) {
      rows.push({ day, tasks: counts.get(day) ?? (first && day >= first ? 0 : null) });
    }
    return { rows, first };
  }
  function setChart(id, option) {
    if (!charts.has(id)) charts.set(id, window.echarts.init($(id)));
    charts.get(id).resize();
    charts.get(id).setOption(option, true);
  }
  function compactDistribution(id, rows, valueText, empty) {
    const root = $(id); root.replaceChildren();
    if (!rows.length && empty) root.append(node('p', empty, 'hint'));
    for (const row of rows) {
      const item = node('div', null, 'ai-distribution-row');
      item.append(node('span', row.name), node('strong', valueText(row)));
      root.append(item);
    }
  }
  async function drawCharts(snapshot) {
    if (!snapshot || !$('usageDetails').open) return;
    const { data, from, to } = snapshot;
    const { rows: days, first } = daySeries(data, from, to);
    const hasTrend = days.some(row => row.tasks !== null);
    const features = (data.features || []).map(row => ({ name: feature(row.key), value: row.tasks }));
    const models = (data.models || []).map(row => ({ name: row.key, value: row.usage_known ? row.input_tokens + row.output_tokens : null }));
    const knownModels = models.filter(row => row.value != null);
    const showModelsChart = knownModels.length > 1;
    $('trend').hidden = !hasTrend;
    $('trendCoverage').textContent = !first ? '暂无采集起点记录'
      : to < first ? '所选日期未采集 · 记录始于 ' + first
      : from < first ? '记录始于 ' + first + '，之前日期未采集' : '';
    $('featurePlot').hidden = features.length < 2;
    $('modelsChart').hidden = !showModelsChart;
    $('chartError').textContent = '';
    compactDistribution('featureSummary', features.length < 2 ? features : [], row => n(row.value) + ' 项', features.length ? '' : '当前筛选无任务');
    compactDistribution('modelSummary', showModelsChart ? models.filter(row => row.value == null) : models,
      row => row.value == null ? '用量未知' : n(row.value) + ' Token', showModelsChart ? '' : '当前筛选无模型记录');
    if (!hasTrend && features.length < 2 && !showModelsChart) return;
    try {
      await AdminAssets.loadCharts();
      if (statistics !== snapshot || !$('usageDetails').open) return;
      if (hasTrend) {
        const trend = {
          color: [AdminChartStyle.colors[0]], aria: { enabled: true },
          grid: { left: 8, right: 12, top: 12, bottom: days.length > 31 ? 44 : 20, containLabel: true },
          xAxis: { type: 'category', data: days.map(row => row.day), axisLabel: { formatter: day => AdminTime.short(day), hideOverlap: true } },
          yAxis: { type: 'value', minInterval: 1, axisLabel: { color: '#667085' } },
          tooltip: { trigger: 'axis', confine: true, renderMode: 'richText', formatter: points => {
            const row = days[points[0]?.dataIndex];
            return row ? row.day + '\n' + (row.tasks == null ? '未采集' : n(row.tasks) + ' 项') : '';
          } },
          series: [{ type: 'bar', name: '任务', data: days.map(row => row.tasks), barMaxWidth: 12, itemStyle: { borderRadius: [4, 4, 0, 0] } }]
        };
        if (days.length > 31) trend.dataZoom = [{ type: 'slider', start: 100 - 3100 / days.length, end: 100, height: 16, bottom: 4 }];
        setChart('trend', trend);
      }
      if (features.length > 1) {
        setChart('featuresChart', AdminChartStyle.ring(features, data.tasks, '任务总数', '项'));
        AdminChartStyle.legend($('featuresLegend'), features);
      }
      if (showModelsChart) {
        const bars = AdminChartStyle.bars(knownModels, 0, { unit: 'Token', labelWidth: 132 });
        bars.series[0].label.formatter = item => n(item.value);
        setChart('modelsChart', bars);
      }
    } catch (error) {
      if (statistics === snapshot) $('chartError').textContent = '图表加载失败，可重新展开或查询重试。';
    }
  }
  function alerts(data) {
    const root = $('alerts'); root.replaceChildren(); const rows = (data.today_usage || {}).features || [];
    (data.policies || []).forEach(control => { const p = control.policy, usage = rows.find(row => row.key === p.feature) || {}; const today = quotaSnapshot ? quotaSnapshot.day : localDate(new Date()); const requests = control.day === today ? control.requests : 0; const messages = [];
      if (p.pause_automatic) messages.push('自动任务已暂停'); if (p.daily_requests > 0 && requests >= p.daily_requests * 0.8) messages.push('今日请求预算已使用 ' + n(requests) + ' / ' + n(p.daily_requests)); if (p.token_alert > 0 && usage.input_tokens + usage.output_tokens >= p.token_alert) messages.push('今日已知 Token 超过提醒阈值'); if (p.cost_alert_cny > 0 && usage.estimated_cny >= p.cost_alert_cny) messages.push('今日已知费用估算超过提醒阈值'); if (usage.failed >= 3 && usage.failed / (usage.failed + usage.success) >= 0.2) messages.push('今日失败比例较高，请查看失败原因'); if (messages.length) root.append(node('div', feature(p.feature) + '：' + messages.join('；'), 'ai-alert'));
    });
  }
  async function loadSettings() {
    const ctrl = replaceController('settings'); try { const data = await request('settings', { signal: ctrl.signal }); if (ctrl.signal.aborted) return; settings = data;
      Object.assign(featureNames, Object.fromEntries((data.features || []).map(f => [f.id, f.name]))); if ($('feature').options.length === 1) (data.features || []).forEach(f => { const option = node('option', f.name); option.value = f.id; $('feature').append(option); }); (data.models || []).forEach(addModel); (data.prices.items || []).forEach(p => addModel(p.model));
      $('notice').textContent = data.durable ? '' : 'AI 记录库连接异常，付费调用已停用。'; $('notice').hidden = data.durable;

      if (!policyDirty && !busySettings) renderPolicies(data.policies || []); if (!pricesDirty && !busySettings) renderPrices(data.prices); alerts(data);
    } catch (error) { if (error.name !== 'AbortError') { $('notice').hidden = false; $('notice').textContent = '配置读取失败，请点击查询重试；已显示数据保留。'; } }
  }
  function numberInput(value, min, max, step) { const input = node('input'); input.type = 'number'; input.min = min; input.max = max; if (Number(min) > 0) input.required = true; input.step = step || '1'; if (value != null) input.value = value; else input.placeholder = '未知'; input.dataset.adminInput = 'ai_settings'; return input; }
  function renderPolicies(controls) {
    const root = $('policies'); root.replaceChildren(); controls.forEach(c => { const p = Object.assign({},c.policy), row = node('tr'); controlPolicies.set(p.feature,p); const title = node('td', feature(p.feature));
      const paused = node('input'); paused.type = 'checkbox'; paused.checked = p.pause_automatic; paused.dataset.adminInput = 'ai_settings'; paused.setAttribute('aria-label', '暂停' + feature(p.feature) + '的自动任务');
      const limits = [numberInput(p.max_concurrent, 1, 16), numberInput(p.token_alert, 0, 1000000000), numberInput(p.cost_alert_cny, 0, 100000, '0.01')]; row.append(title); const pauseCell = node('td'); const pauseLabel = node('label', '', 'form-check'); pauseLabel.append(paused, document.createTextNode('暂停自动')); if (['track_description','track_shape'].includes(p.feature)) {pauseCell.append(node('span','仅手动入口','hint'));} else {pauseCell.append(pauseLabel);} row.append(pauseCell); limits.forEach((input, i) => { input.setAttribute('aria-label', feature(p.feature) + ['最大并发', 'Token 提醒阈值', '费用提醒阈值'][i]); const td = node('td'); td.append(input); row.append(td); });
      [paused].concat(limits).forEach(input => input.addEventListener('change', () => { policyDirty = true; })); const action = node('td'); const save = button('保存', async () => {
        if (busySettings) return; if (limits.some(input => !input.reportValidity())) return; busySettings = true; save.disabled = true;
        try { await request('policies/' + encodeURIComponent(p.feature), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(Object.assign({}, p, { pause_automatic: paused.checked, max_concurrent: Number(limits[0].value), token_alert: Number(limits[1].value), cost_alert_cny: Number(limits[2].value) })) }); $('policyMessage').textContent = feature(p.feature) + '配置已保存。'; p.revision++; policyDirty = true; }
        catch (error) { showError('policyMessage', error); } finally { busySettings = false; save.disabled = false; loadSettings(); loadQuotas(true); }
      }, 'ai_settings'); action.append(save); row.append(action); root.append(row);
    });
  }
  function renderPrices(book) {
    const root = $('prices'); root.replaceChildren(); root.dataset.revision = book.revision; const models = new Set((settings.models || []).concat((book.items || []).map(p => p.model)));
    models.forEach(model => { const old = (book.items || []).find(p => p.model === model && p.provider === 'moonshot') || { model, provider: 'moonshot' }; const row = node('tr'); row.dataset.model = model; row.append(node('td', 'Moonshot / ' + model)); ['input_per_million', 'cached_per_million', 'output_per_million', 'search_per_call'].forEach(key => { const td = node('td'), input = numberInput(old[key], 0, 1000000, 'any'); input.dataset.priceKey = key; input.setAttribute('aria-label', model + ' ' + key); input.addEventListener('input', () => { pricesDirty = true; }); td.append(input); row.append(td); }); row.append(node('td', old.version ? 'v' + old.version + ' · ' + dateTime(old.updated_at) : '未配置')); root.append(row); });
  }
  $('savePrices').addEventListener('click', async () => { if (!settings || busySettings) return; const rows = Array.from($('prices').rows); if (rows.some(row => Array.from(row.querySelectorAll('input')).some(input => !input.reportValidity()))) return; const items = rows.map(row => { const item = { provider: 'moonshot', model: row.dataset.model }; row.querySelectorAll('input').forEach(input => { item[input.dataset.priceKey] = input.value === '' ? null : Number(input.value); }); return item; }); busySettings = true;
    try { await request('prices', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ revision: Number($('prices').dataset.revision), items }) }); pricesDirty = false; $('priceMessage').textContent = '价格配置已保存，仅用于后续请求。'; }
    catch (error) { showError('priceMessage', error); } finally { busySettings = false; loadSettings(); }
  });
  async function openDetail(id) {
    modalTaskID = id;
    detailController?.abort();
    detailController = new AbortController();
    $('detailTitle').textContent = 'AI 任务详情';
    $('detail').replaceChildren(node('p', '正在读取…', 'hint'));
    modal.show();
    await loadDetail(id, detailController.signal);
  }
  async function loadDetail(id, signal) {
    try { const t = await request('tasks/' + encodeURIComponent(id), { signal }); if (signal.aborted || modalTaskID !== id) return; const root = $('detail'); root.replaceChildren(); const grid = node('div', null, 'ai-detail-grid');
      $('detailTitle').textContent = feature(t.feature);
      const fields = [['模型', t.model || '—'], ['触发', [t.mode === 'manual' ? '手动' : '自动', t.actor].filter(Boolean).join(' · ')], ['创建时间', dateTime(t.created_at)], ['耗时', seconds(t.finished_at ? t.duration_ms : Date.now() - new Date(t.created_at).getTime())]];
      fields.forEach(([label, value]) => { const field = node('div', null, 'ai-detail-field'); field.append(node('strong', label), node('span', value)); grid.append(field); });
      const outcome = node('div', null, 'ai-task-outcome');
      outcome.append(node('strong', states[t.status] || t.status, 'ai-state ai-state-' + t.status));
      if (t.adoption && t.adoption !== 'not_generated') outcome.append(node('span', adoptions[t.adoption] || t.adoption, 'text-secondary'));
      root.append(outcome, objectLink(t));
      const failed = t.status === 'failed' || t.status === 'interrupted';
      const failedCall = failed && (t.calls || []).findLast(call => errorNames[call.error_code]);
      const reason = t.error || (failedCall && errorNames[failedCall.error_code]);
      if (reason || failed) root.append(node('p', reason || '暂无已记录的失败原因', 'ai-alert'));
      const actions = node('div', null, 'ai-detail-actions');
      taskActions(t, actions, true);
      if (t.retry_of) actions.append(button('查看原任务', () => openDetail(t.retry_of)));
      if (t.retry_task_id) actions.append(button('查看重试任务', () => openDetail(t.retry_task_id)));
      if (actions.childElementCount) root.append(actions);
      const technical = node('details', null, 'ai-disclosure'); technical.append(node('summary', '请求与技术明细'));
      [['任务编号', t.id], ['触发原因', t.reason], ['执行阶段', stages[t.stage] || t.stage], ['规则版本', t.rule_version], ['输入版本', t.input_version], ['输入摘要', t.input_summary]].forEach(([label, value]) => { if (value) technical.append(node('p', label + '：' + value, 'hint')); });
      const scroll = node('div', null, 'table-responsive'), table = node('table', null, 'data ai-request-table'); const head = node('thead'), hr = node('tr'); ['阶段', '开始时间', '状态 / 错误', '耗时', '输入 / 输出 / 缓存 Token', '预估费用 / 价格版本'].forEach(text => hr.append(node('th', text))); head.append(hr); table.append(head); const body = node('tbody'); (t.calls || []).forEach(call => { const row = node('tr'); [stages[call.kind] || call.kind, dateTime(call.started_at), (states[call.status] || call.status) + (call.http_status ? ' · HTTP ' + call.http_status : '') + (call.error_code ? ' · ' + (errorNames[call.error_code] || call.error_code) : ''), seconds(call.duration_ms), [call.input_tokens, call.output_tokens, call.cached_tokens].map(v => v == null ? '未知' : n(v)).join(' / '), (call.estimated_cny == null ? '未知' : '≈ ¥' + amount(call.estimated_cny)) + ' · ' + (call.price_version ? 'v' + call.price_version : '未配置')].forEach(text => row.append(node('td', text))); if(call.price_snapshot){const price=call.price_snapshot,detail=node('details'),summary=node('summary','当时单价');detail.append(summary);[['输入',price.input_per_million],['缓存命中',price.cached_per_million],['输出',price.output_per_million],['搜索/次',price.search_per_call]].forEach(([label,value])=>detail.append(node('p',label+'：'+(value==null?'未知':amount(value))+' 元'+(label==='搜索/次'?' / 次':' / 百万 Token'),'hint')));row.lastChild.append(detail);} body.append(row); }); table.append(body); scroll.append(table); technical.append(scroll);
      if (t.result) {
        let result; try { result = JSON.parse(t.result); } catch (_) { /* Preserve non-JSON historical output below. */ }
        const output = node('pre', null, 'ai-detail-result'); output.textContent = result ? JSON.stringify(result, null, 2) : t.result;
        technical.append(node('h3', '完整结果'), output);
        if (result && (result.description || result.shape_label || result.decision_summary)) {
          root.append(node('h3', '生成内容'));
          if (result.description) root.append(node('p', result.description, 'ai-result-text'));
          if (result.shape_label) root.append(node('p', '图案：' + result.shape_label));
          if (result.decision_summary) root.append(node('p', result.decision_summary, 'ai-result-text'));
        } else root.append(node('p', '生成结果可展开下方明细查看。', 'hint'));
        if (t.feature === 'track_description' && t.status === 'success' && t.adoption !== 'applied' && result && result.description && result.source_version && can('tracks', true)) {
          const input = node('textarea'); input.className = 'form-control'; input.rows = 4; input.maxLength = 4000; input.value = result.description; input.setAttribute('aria-label', '待确认的路线说明'); const msg = node('p', '可修改建议后单独保存路线说明，其他字段不变。', 'hint');
          const apply = button('一键代入并保存路线说明', async () => { if (!input.value.trim()) return; apply.disabled = true; try { const response = await fetch('/admin/api/tracks/' + encodeURIComponent(t.object_id) + '/description', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, signal, body: JSON.stringify({ description: input.value.trim(), source_version: result.source_version, task_id: t.id }) }); const data = await response.json(); if (!response.ok || !data.data || !data.data.saved) throw new Error(data.error || '保存结果未确认，请核实'); msg.textContent = '路线说明已保存。'; apply.remove(); refresh(false); } catch (error) { if (!signal.aborted) msg.textContent = error.message; } finally { apply.disabled = false; } }, 'tracks'); root.append(input, msg, apply);
        }
      }
      root.append(grid, technical);
    } catch (error) { if (!signal.aborted) $('detail').replaceChildren(node('p', error.message, 'text-danger'), button('重试读取', () => loadDetail(id, signal))); }
  }
  $('detailModal').addEventListener('admin:dialog-closed', () => { modalTaskID = ''; if (detailController) detailController.abort(); $('detail').replaceChildren(); });
  function refresh(reset) {
    if (reset) { cursor = ''; nextCursor = ''; history = []; page = 1; }
    $('pageError').textContent = '';
    loadTasks(); loadStatistics(); loadActive(false); loadSettings(); loadQuotas(true);
  }
  $('period').addEventListener('change', () => applyFilters());
  ['from', 'to', ...filterIDs].forEach(id => $(id).addEventListener('change', () => applyFilters()));
  $('q').addEventListener('input', updateSearchHint);
  $('q').addEventListener('keydown', event => {
    if (event.key === 'Enter' && !event.isComposing) { event.preventDefault(); applyFilters(true); }
  });
  $('refresh').addEventListener('click', () => applyFilters(true));
  document.querySelectorAll('[data-task-status]').forEach(button => button.addEventListener('click', () => {
    const status = button.dataset.taskStatus;
    $('status').value = appliedFilters.get('status') === status ? '' : status;
    applyFilters();
  }));
  $('prev').addEventListener('click', () => {
    if (history.length) loadTasks({ cursor: history[history.length - 1], history: history.slice(0, -1), page: page - 1 });
  });
  $('next').addEventListener('click', () => {
    if (nextCursor) loadTasks({ cursor: nextCursor, history: [...history, cursor], page: page + 1 });
  });
  $('toggleActive').addEventListener('click', () => { activeOpen = !activeOpen; showActiveBody(); });
  $('moreActive').addEventListener('click', () => loadActive(true));
  $('usageDetails').addEventListener('toggle', () => {
    if ($('usageDetails').open) requestAnimationFrame(() => drawCharts(statistics));
  });
  $('openQuota').addEventListener('click', () => {
    $('settingsSection').open = true;
    $('quotaSection').scrollIntoView({ block: 'start' });
    $('refreshQuota').focus({ preventScroll: true });
  });
  $('openPricing').addEventListener('click', () => {
    if (!can('ai_settings', true)) return;
    $('settingsSection').open = true;
    $('pricingSection').open = true;
    $('pricingSection').scrollIntoView({ block: 'start' });
    $('pricingSection').querySelector('summary').focus({ preventScroll: true });
  });
  $('reloadSettings').addEventListener('click', async () => { if (busySettings) return; if ((policyDirty || pricesDirty) && !await AdminUI.confirm({title:'重新载入配置',message:'将放弃尚未保存的预算和价格修改，读取服务端最新配置。'})) return; policyDirty=false; pricesDirty=false; await loadSettings(); });
  $('clearFilters').addEventListener('click', () => { ['provider', 'mode', 'status', 'adoption'].forEach(id => { $(id).value = ''; }); applyFilters(); });
  $('refreshQuota').addEventListener('click',()=>loadQuotas(true));
  let refreshTicks=0;
  const timer = setInterval(() => {
    if (document.hidden || !$('autoRefresh').checked) return;
    loadQuotas(false);
    if (!appliedFilters.has('from')) return;
    if (!activeLoading) loadActive(false);
    if (++refreshTicks % 3 === 0) {
      if (!statisticsLoading) loadStatistics();
      if (!tasksLoading) loadTasks();
    }
  }, 5000);
  const resize = new ResizeObserver(() => charts.forEach(chart => {
    if (chart.getDom().clientWidth && chart.getDom().clientHeight) chart.resize();
  }));
  ['trend', 'featuresChart', 'modelsChart'].forEach(id => resize.observe($(id)));
  window.addEventListener('pagehide', () => { clearInterval(timer); resize.disconnect(); Object.values(controllers).forEach(ctrl => ctrl.abort()); if (detailController) detailController.abort(); charts.forEach(chart => chart.dispose()); });
  applyFilters(true);
  if (!appliedFilters.has('from')) { loadSettings(); loadQuotas(true); }
  const deepTask = new URLSearchParams(location.search).get('task_id'); if (deepTask) openDetail(deepTask);
})();
