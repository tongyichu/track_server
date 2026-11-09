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
  const dateTime = value => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—';
  const seconds = value => (Number(value || 0) / 1000).toFixed(1) + ' 秒';
  const feature = value => featureNames[value] || value;
  const node = (tag, text, className) => { const el = document.createElement(tag); if (text != null) el.textContent = String(text); if (className) el.className = className; return el; };
  const can = (module, write) => window.AdminAccess && AdminAccess.can(module, !!write);
  let settings = null, statistics = null, cursor = '', nextCursor = '', history = [], page = 1, activeCursor = '', activeRows = [], modalTaskID = '', detailController = null;
  let policyDirty = false, pricesDirty = false, busySettings = false;
  const controllers = {}, charts = new Map(), operations = new Set();
  const modal = new tabler.Modal($('detailModal'));
  function localDate(d) { return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0'); }
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
      if (error.name !== 'AbortError') $('quotaHint').textContent = '配额读取失败，请刷新重试' + (quotaSnapshot ? '；保留上次数据' : '');
    } finally {
      if (controllers.quotas === ctrl) { quotaLoading = false; $('refreshQuota').disabled = false; }
    }
  }
  function quotaRow(id) {
    if (quotaNodes.has(id)) return quotaNodes.get(id);
    if (!quotaNodes.size) $('quotaRows').replaceChildren();
    const row = node('tr'), title = node('strong', feature(id)), meta = node('small', '', 'ai-quota-meta');
    const name = node('td'); name.append(title, meta);
    const used = node('td'), limit = node('td'), remaining = node('td'), state = node('td'), load = node('td'), action = node('td');
    const caption = node('span'), progress = node('div', null, 'progress'), bar = node('div', null, 'progress-bar'); progress.append(bar); state.append(caption, progress);
    const actions = node('div', null, 'ai-quota-actions');
    const edit = button('调整配额', () => editQuota(id), 'ai_settings');
    const unlimited = button('设为不限', () => editQuota(id, 0), 'ai_settings');
    actions.append(edit, unlimited); action.append(actions);
    row.append(name, used, limit, remaining, state, load, action); $('quotaRows').append(row);
    const item = { row, title, meta, used, limit, remaining, caption, progress, bar, load, edit, unlimited };
    quotaNodes.set(id, item); return item;
  }
  function renderQuotas(data) {
    const items = data.items || [], existing = new Set(items.map(item => item.feature));
    quotaNodes.forEach((item, id) => { if (!existing.has(id)) { item.row.remove(); quotaNodes.delete(id); } });
    const total = items.reduce((sum, item) => sum + item.used, 0), unlimited = items.filter(item => item.unlimited).length;
    const exhausted = items.filter(item => !item.unlimited && item.remaining === 0).length;
    $('quotaSummary').replaceChildren(metric('今日已计额度', n(total) + ' 次', '含失败和已预留请求'), metric('不限配额', n(unlimited) + ' 个功能', '继续记录用量'), metric('已设日上限', n(items.length - unlimited) + ' 个功能', '各功能分别计数'), metric('今日配额已用尽', n(exhausted) + ' 个功能', '调高或设为不限可恢复后续请求'));
    const clock = new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(data.as_of));
    $('quotaHint').textContent = data.day + ' · 北京时间 ' + clock + ' 更新 · 每日 00:00 重新计数';
    items.forEach(item => {
      const view = quotaRow(item.feature), policy = item.policy;
      view.title.textContent = feature(item.feature); view.used.textContent = n(item.used) + ' 次';
      view.limit.textContent = item.unlimited ? '不限' : n(policy.daily_requests) + ' 次 / 天';
      view.remaining.textContent = item.unlimited ? '不限' : n(item.remaining) + ' 次';
      const actor = policy.updated_by && policy.updated_by.startsWith('system:') ? '系统' : policy.updated_by;
      view.meta.textContent = (policy.pause_automatic ? '自动已暂停 · ' : '') + (actor ? '最近调整：' + actor : '默认不限');
      view.meta.title = actor ? dateTime(policy.updated_at) : '';
      view.load.textContent = n(item.active) + ' / ' + n(policy.max_concurrent);
      view.progress.hidden = item.unlimited;
      if (item.unlimited) { view.caption.textContent = '持续计量'; view.progress.removeAttribute('role'); }
      else {
        const percent = Math.min(100, item.used * 100 / policy.daily_requests);
        view.caption.textContent = item.remaining === 0 ? '配额已用尽' : '已用 ' + percent.toFixed(1) + '%';
        view.bar.style.width = percent + '%'; view.bar.classList.toggle('exhausted', item.remaining === 0);
        view.progress.setAttribute('role', 'progressbar'); view.progress.setAttribute('aria-label', feature(item.feature) + '配额使用情况');
        view.progress.setAttribute('aria-valuemin', '0'); view.progress.setAttribute('aria-valuemax', String(policy.daily_requests));
        view.progress.setAttribute('aria-valuenow', String(Math.min(item.used, policy.daily_requests))); view.progress.setAttribute('aria-valuetext', '已用' + item.used + '次，剩余' + item.remaining + '次');
      }
      view.edit.disabled = !data.durable; view.unlimited.disabled = !data.durable; view.unlimited.hidden = item.unlimited;
    });
    if (!items.length) {const row=node('tr'),cell=node('td','暂无可用功能配额','hint');cell.colSpan=7;row.append(cell);$('quotaRows').replaceChildren(row);}
  }
  function editQuota(id, preset) {
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
          return { beforeFields: intro, afterFields: note, fields: [{ name: 'daily_requests', label: '每日请求配额（0＝不限）', type: 'number', value: preset != null ? preset : quotaDrafts.has(id) ? quotaDrafts.get(id) : baseline.daily_requests, min: 0, step: 1, required: true }] };
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

  function setPeriod() { if ($('period').value === 'custom') return; const today = new Date(); const from = new Date(today); from.setDate(from.getDate() - (Number($('period').value) || 1) + 1); $('from').value = localDate(from); $('to').value = localDate(today); }
  setPeriod();
  function query(active) {
    const params = new URLSearchParams();
    ['from', 'to', 'feature', 'provider', 'model', 'mode', 'status', 'adoption', 'q'].forEach(id => { const value = $(id).value.trim(); if (value) params.set(id, value); });
    if (active) { params.set('active', 'true'); params.delete('status'); params.delete('adoption'); }
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
  function taskActions(t, target) {
    target.append(button('详情', () => openDetail(t.id)));
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
  async function loadTasks() {
    const ctrl = replaceController('tasks'), params = query(false); tasksLoading=true; if (cursor) params.set('cursor', cursor);
    try { const data = await request('tasks?' + params, { signal: ctrl.signal }); if (ctrl.signal.aborted) return; renderTasks(data.items || []); nextCursor = data.next_cursor || ''; $('total').textContent = '共 ' + n(data.total) + ' 项'; $('prev').disabled = history.length === 0; $('next').disabled = !nextCursor; $('pageHint').textContent = '第 ' + page + ' 页'; }
    catch (error) { showError('pageError', error); }
    finally {if(controllers.tasks===ctrl)tasksLoading=false;}
  }
  function renderTasks(items) {
    const body = $('tasks'); body.replaceChildren(); if (!items.length) { const td = node('td', '当前条件下暂无任务记录', 'hint'); td.colSpan = 6; const row = node('tr'); row.append(td); body.append(row); return; }
    items.forEach(t => {
      const row = node('tr');
      const time = node('td'); time.append(node('span', dateTime(t.created_at)), node('small', t.id));
      const business = node('td'); business.append(node('strong', feature(t.feature)), node('br'), objectLink(t));
      const source = node('td', (t.mode === 'manual' ? '手动' : '自动') + ' · ' + t.actor); source.append(node('small', t.reason), node('small', t.provider + ' / ' + t.model));
      const status = node('td'); status.append(node('span', states[t.status] || t.status, 'ai-state ai-state-' + t.status), node('small', adoptions[t.adoption] || t.adoption), node('small', t.finished_at ? seconds(t.duration_ms) : stages[t.stage] || t.stage)); if (t.error) status.append(node('small', t.error));
      const usage = node('td', n(t.requests) + ' 次请求'); usage.append(node('small', t.usage_known ? n(t.input_tokens) + ' 输入 / ' + n(t.output_tokens) + ' 输出 Token' : 'Token 暂无记录'), node('small', t.cost_known ? '已知部分 ≈ ¥' + amount(t.estimated_cny) : '费用未知'));
      const actions = node('td'); taskActions(t, actions); if (t.retry_of) actions.append(button('原任务', () => openDetail(t.retry_of))); if (t.retry_task_id) actions.append(button('重试任务', () => openDetail(t.retry_task_id)));
      row.append(time, business, source, status, usage, actions); body.append(row);
    });
  }
  async function loadActive(append) {
    const ctrl = replaceController('active'), params = query(true); if (append && activeCursor) params.set('cursor', activeCursor);
    try { const data = await request('tasks?' + params, { signal: ctrl.signal }); if (ctrl.signal.aborted) return; activeCursor = data.next_cursor || ''; activeRows = append ? activeRows.concat(data.items || []) : data.items || []; $('activeHint').textContent = n(data.total) + ' 项'; $('moreActive').hidden = !activeCursor; const root = $('active'); root.replaceChildren();
      if (!activeRows.length) root.append(node('p', '当前没有正在执行或排队的任务', 'hint'));
      activeRows.forEach(t => { const card = node('article', null, 'ai-active-card'); card.append(node('strong', feature(t.feature)), node('p')); card.lastChild.append(objectLink(t)); card.append(node('p', (t.mode === 'manual' ? '手动' : '自动') + ' · ' + t.actor + ' · ' + t.model), node('p', t.reason, 'hint'), node('p', (states[t.status] || t.status) + ' · ' + (stages[t.stage] || t.stage) + ' · 已等待/运行 ' + seconds(Date.now() - new Date(t.created_at).getTime()))); const actions = node('div'); taskActions(t, actions); card.append(actions); root.append(card); }); }
    catch (error) { $('activeHint').textContent = error.name === 'AbortError' ? $('activeHint').textContent : '读取失败，保留上次数据'; }
  }
  function metric(title, value, hint) { const card = node('div', null, 'ai-metric'); card.append(node('span', title), node('strong', value), node('small', hint)); return card; }
  async function loadStatistics() {
    const ctrl = replaceController('statistics'); statisticsLoading=true;
    try { const data = await request('statistics?' + query(false), { signal: ctrl.signal }); if (ctrl.signal.aborted) return; statistics = data; const root = $('stats'); root.replaceChildren();
      root.append(metric('任务 / 请求', n(data.tasks) + ' / ' + n(data.requests), '一个任务可包含搜索、生成等多个请求'), metric('执行中 / 排队', n(data.running) + ' / ' + n(data.queued), '所选筛选范围内'), metric('成功率', data.success + data.failed ? ((data.success * 100) / (data.success + data.failed)).toFixed(1) + '%' : '—', n(data.failed) + ' 项失败；中断不计入成功率'), metric('输入 / 输出 Token', n(data.input_tokens) + ' / ' + n(data.output_tokens), n(data.usage_known) + ' 次请求返回了用量，其余不补零'), metric('预估费用（已知部分）', data.cost_known ? '¥' + amount(data.estimated_cny) : '未知', n(data.cost_known) + ' / ' + n(data.requests) + ' 次请求可估算，非供应商账单'), metric('平均耗时 / 已采用', (data.finished ? seconds(data.duration_ms / data.finished) : '—') + ' / ' + n(data.adopted), '耗时包含排队；采用量按当前任务结果计'));
      $('statHint').textContent = '北京时间 · ' + (data.first_recorded_at ? '记录始于 ' + dateTime(data.first_recorded_at) : '上线后开始记录，历史未采集数据不可追溯');
      renderBreakdown(data);
      (data.models || []).forEach(row => addModel(row.key.split(' / ').slice(1).join(' / '))); drawCharts(data); }
    catch (error) { if (error.name !== 'AbortError') $('statHint').textContent = '统计读取失败，请点击查询重试'; }
    finally {if(controllers.statistics===ctrl)statisticsLoading=false;}
  }
  function renderBreakdown(data) {
    const root=$('breakdown');root.replaceChildren();
    [['features','功能'],['models','模型']].forEach(([key,caption]) => (data[key]||[]).forEach(group => {const row=node('tr');[caption+' · '+(key==='features'?feature(group.key):group.key),n(group.tasks)+' / '+n(group.requests),n(group.success)+' / '+n(group.failed),n(group.input_tokens)+' / '+n(group.output_tokens)+(group.usage_known?'':'（未返回用量）'),group.cost_known?'≈ ¥'+amount(group.estimated_cny)+'（'+n(group.cost_known)+'次可估算）':'未知',n(group.adopted)].forEach(value=>row.append(node('td',value)));root.append(row);}));
    if(!root.children.length){const row=node('tr'),cell=node('td','当前周期暂无数据','hint');cell.colSpan=6;row.append(cell);root.append(row);}
  }
  function addModel(value) { if (value && !Array.from($('model').options).some(option => option.value === value)) { const option = node('option', value); option.value = value; $('model').append(option); } }
  async function drawCharts(data) {
    try { const ec = await AdminAssets.loadCharts(); if (statistics !== data) return; ['trend', 'featuresChart', 'modelsChart'].forEach(id => { if (!charts.has(id)) charts.set(id, ec.init($(id))); });
      const days = data.days || []; const trend = AdminChartStyle.bars(days.map(row => ({ name: row.key, value: row.tasks })), data.tasks, { unit: '项' }); if (days.length > 10) trend.dataZoom = [{ type: 'inside', yAxisIndex: 0, start: Math.max(0, 100 - 1000 / days.length), end: 100 }]; charts.get('trend').setOption(trend, true);
      const features = (data.features || []).map(row => ({ name: feature(row.key), value: row.tasks })); charts.get('featuresChart').setOption(AdminChartStyle.ring(features, data.tasks, '任务总数', '项'), true); AdminChartStyle.legend($('featuresLegend'), features);
      const models = (data.models || []).map(row => ({ name: row.key, value: row.input_tokens + row.output_tokens })); charts.get('modelsChart').setOption(AdminChartStyle.bars(models, data.input_tokens + data.output_tokens, { unit: 'Token', labelWidth: 150 }), true);
    } catch (_) { if (statistics === data) $('statHint').textContent += ' · 图表加载失败，查询可重试'; }
  }
  function alerts(data) {
    const root = $('alerts'); root.replaceChildren(); const rows = (data.today_usage || {}).features || [];
    (data.policies || []).forEach(control => { const p = control.policy, usage = rows.find(row => row.key === p.feature) || {}; const today = quotaSnapshot ? quotaSnapshot.day : localDate(new Date()); const requests = control.day === today ? control.requests : 0; const messages = [];
      if (p.pause_automatic) messages.push('自动任务已暂停'); if (p.daily_requests > 0 && requests >= p.daily_requests * 0.8) messages.push('今日请求预算已使用 ' + n(requests) + ' / ' + n(p.daily_requests)); if (p.token_alert > 0 && usage.input_tokens + usage.output_tokens >= p.token_alert) messages.push('今日已知 Token 超过提醒阈值'); if (p.cost_alert_cny > 0 && usage.estimated_cny >= p.cost_alert_cny) messages.push('今日已知费用估算超过提醒阈值'); if (usage.failed >= 3 && usage.failed / (usage.failed + usage.success) >= 0.2) messages.push('今日失败比例较高，请查看失败原因'); if (usage.finished >= 3 && usage.duration_ms / usage.finished >= 60000) messages.push('今日平均任务耗时超过一分钟'); if (messages.length) root.append(node('div', feature(p.feature) + '：' + messages.join('；'), 'ai-alert'));
    });
  }
  async function loadSettings() {
    const ctrl = replaceController('settings'); try { const data = await request('settings', { signal: ctrl.signal }); if (ctrl.signal.aborted) return; settings = data;
      Object.assign(featureNames, Object.fromEntries((data.features || []).map(f => [f.id, f.name]))); if ($('feature').options.length === 1) (data.features || []).forEach(f => { const option = node('option', f.name); option.value = f.id; $('feature').append(option); }); (data.models || []).forEach(addModel); (data.prices.items || []).forEach(p => addModel(p.model));
      $('notice').textContent = data.durable ? '用量从功能上线后开始记录；费用为按配置单价计算的估算。共享日次数上限已移除，各功能的每日请求上限填 0 表示不限。' : '当前未连接持久化 AI 记录库，新的 AI 付费调用已停用。';
      if (!policyDirty && !busySettings) renderPolicies(data.policies || []); if (!pricesDirty && !busySettings) renderPrices(data.prices); alerts(data);
    } catch (error) { if (error.name !== 'AbortError') $('notice').textContent = '配置读取失败，请点击查询重试；已显示数据保留。'; }
  }
  function numberInput(value, min, max, step) { const input = node('input'); input.type = 'number'; input.min = min; input.max = max; if (Number(min) > 0) input.required = true; input.step = step || '1'; if (value != null) input.value = value; else input.placeholder = '未知'; input.dataset.adminInput = 'ai_settings'; return input; }
  function renderPolicies(controls) {
    const root = $('policies'); root.replaceChildren(); controls.forEach(c => { const p = Object.assign({},c.policy), row = node('tr'); controlPolicies.set(p.feature,p); const title = node('td', feature(p.feature));
      const paused = node('input'); paused.type = 'checkbox'; paused.checked = p.pause_automatic; paused.dataset.adminInput = 'ai_settings'; paused.setAttribute('aria-label', '暂停' + feature(p.feature) + '的自动任务');
      const limits = [numberInput(p.max_concurrent, 1, 16), numberInput(p.token_alert, 0, 1000000000), numberInput(p.cost_alert_cny, 0, 100000, '0.01')]; row.append(title); const pauseCell = node('td'); const pauseLabel = node('label'); pauseLabel.append(paused, document.createTextNode(' 暂停自动')); if (['track_description','track_shape'].includes(p.feature)) {pauseCell.append(node('span','仅手动入口','hint'));} else {pauseCell.append(pauseLabel);} row.append(pauseCell); limits.forEach((input, i) => { input.setAttribute('aria-label', feature(p.feature) + ['最大并发', 'Token 提醒阈值', '费用提醒阈值'][i]); const td = node('td'); td.append(input); row.append(td); });
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
  async function openDetail(id) { modalTaskID = id; if (detailController) detailController.abort(); detailController = new AbortController(); $('detail').replaceChildren(node('p', '正在读取…', 'hint')); modal.show(); await loadDetail(id, detailController.signal); }
  async function loadDetail(id, signal) {
    try { const t = await request('tasks/' + encodeURIComponent(id), { signal }); if (signal.aborted || modalTaskID !== id) return; const root = $('detail'); root.replaceChildren(); const grid = node('div', null, 'ai-detail-grid');
      const fields = [['任务编号', t.id], ['功能', feature(t.feature)], ['触发', (t.mode === 'manual' ? '手动' : '自动') + ' · ' + t.actor + ' · ' + t.reason], ['模型', t.provider + ' / ' + t.model], ['状态', (states[t.status] || t.status) + ' · ' + (stages[t.stage] || t.stage)], ['结果去向', adoptions[t.adoption] || t.adoption], ['创建 / 开始', dateTime(t.created_at) + ' / ' + dateTime(t.started_at)], ['结束 / 总耗时', dateTime(t.finished_at) + ' / ' + seconds(t.finished_at ? t.duration_ms : Date.now() - new Date(t.created_at).getTime())], ['规则版本', t.rule_version], ['输入版本', t.input_version], ['失败说明', t.error || '—'], ['输入摘要', t.input_summary || '—']]; fields.forEach(([label, value]) => { const field = node('div', null, 'ai-detail-field'); field.append(node('strong', label), node('span', value)); grid.append(field); }); root.append(grid, objectLink(t));
      const actions = node('div', null, 'ai-detail-actions'); taskActions(t, actions); if (t.retry_of) actions.append(button('查看原任务', () => openDetail(t.retry_of))); if (t.retry_task_id) actions.append(button('查看重试任务', () => openDetail(t.retry_task_id))); root.append(actions);
      const scroll = node('div', null, 'table-responsive'), table = node('table', null, 'data ai-request-table'); const head = node('thead'), hr = node('tr'); ['阶段', '开始时间', '状态 / 错误', '耗时', '输入 / 输出 / 缓存 Token', '预估费用 / 价格版本'].forEach(text => hr.append(node('th', text))); head.append(hr); table.append(head); const body = node('tbody'); (t.calls || []).forEach(call => { const row = node('tr'); [stages[call.kind] || call.kind, dateTime(call.started_at), (states[call.status] || call.status) + (call.http_status ? ' · HTTP ' + call.http_status : '') + (call.error_code ? ' · ' + (errorNames[call.error_code] || call.error_code) : ''), seconds(call.duration_ms), [call.input_tokens, call.output_tokens, call.cached_tokens].map(v => v == null ? '未知' : n(v)).join(' / '), (call.estimated_cny == null ? '未知' : '≈ ¥' + amount(call.estimated_cny)) + ' · ' + (call.price_version ? 'v' + call.price_version : '未配置')].forEach(text => row.append(node('td', text))); if(call.price_snapshot){const price=call.price_snapshot,detail=node('details'),summary=node('summary','当时单价');detail.append(summary);[['输入',price.input_per_million],['缓存命中',price.cached_per_million],['输出',price.output_per_million],['搜索/次',price.search_per_call]].forEach(([label,value])=>detail.append(node('p',label+'：'+(value==null?'未知':amount(value))+' 元'+(label==='搜索/次'?' / 次':' / 百万 Token'),'hint')));row.lastChild.append(detail);} body.append(row); }); table.append(body); scroll.append(table); root.append(node('h3', '请求明细'), scroll);
      if (t.result) { root.append(node('h3', '生成结果')); const output = node('pre', null, 'ai-detail-result'); let result; try { result = JSON.parse(t.result); output.textContent = JSON.stringify(result, null, 2); } catch (_) { output.textContent = t.result; } root.append(output);
        if (t.feature === 'track_description' && t.status === 'success' && t.adoption !== 'applied' && result && result.description && result.source_version && can('tracks', true)) {
          const input = node('textarea'); input.className = 'form-control'; input.rows = 4; input.maxLength = 4000; input.value = result.description; input.setAttribute('aria-label', '待确认的路线说明'); const msg = node('p', '可修改建议后单独保存路线说明，其他字段不变。', 'hint');
          const apply = button('一键代入并保存路线说明', async () => { if (!input.value.trim()) return; apply.disabled = true; try { const response = await fetch('/admin/api/tracks/' + encodeURIComponent(t.object_id) + '/description', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, signal, body: JSON.stringify({ description: input.value.trim(), source_version: result.source_version, task_id: t.id }) }); const data = await response.json(); if (!response.ok || !data.data || !data.data.saved) throw new Error(data.error || '保存结果未确认，请核实'); msg.textContent = '路线说明已保存。'; apply.remove(); refresh(false); } catch (error) { if (!signal.aborted) msg.textContent = error.message; } finally { apply.disabled = false; } }, 'tracks'); root.append(input, msg, apply);
        }
      }
    } catch (error) { if (!signal.aborted) $('detail').replaceChildren(node('p', error.message, 'text-danger'), button('重试读取', () => loadDetail(id, signal))); }
  }
  $('detailModal').addEventListener('hidden.bs.modal', () => { modalTaskID = ''; if (detailController) detailController.abort(); $('detail').replaceChildren(); });
  function refresh(reset) { if (reset) { cursor = ''; nextCursor = ''; history = []; page = 1; } $('pageError').textContent = ''; loadTasks(); loadStatistics(); loadActive(false); loadSettings(); loadQuotas(true); }
  $('period').addEventListener('change', () => { setPeriod(); refresh(true); }); ['from', 'to'].forEach(id => $(id).addEventListener('change', () => { $('period').value = 'custom'; refresh(true); })); ['feature', 'provider', 'model', 'mode', 'status', 'adoption'].forEach(id => $(id).addEventListener('change', () => refresh(true))); $('q').addEventListener('keydown', event => { if (event.key === 'Enter') refresh(true); }); $('refresh').addEventListener('click', () => refresh(true)); $('prev').addEventListener('click', () => { cursor = history.pop() || ''; page--; loadTasks(); }); $('next').addEventListener('click', () => { if (!nextCursor) return; history.push(cursor); cursor = nextCursor; page++; loadTasks(); }); $('moreActive').addEventListener('click', () => loadActive(true));
  $('reloadSettings').addEventListener('click', async () => { if (busySettings) return; if ((policyDirty || pricesDirty) && !await AdminUI.confirm({title:'重新载入配置',message:'将放弃尚未保存的预算和价格修改，读取服务端最新配置。'})) return; policyDirty=false; pricesDirty=false; await loadSettings(); });
  $('refreshQuota').addEventListener('click',()=>loadQuotas(true));
  let refreshTicks=0;
  const timer = setInterval(() => { if (document.hidden || !$('autoRefresh').checked) return; loadActive(false); loadQuotas(false); if(++refreshTicks%3===0){if(!statisticsLoading)loadStatistics();if(!tasksLoading)loadTasks();} }, 5000);
  const resize = new ResizeObserver(() => charts.forEach(chart => chart.resize())); ['trend', 'featuresChart', 'modelsChart'].forEach(id => resize.observe($(id)));
  window.addEventListener('pagehide', () => { clearInterval(timer); resize.disconnect(); Object.values(controllers).forEach(ctrl => ctrl.abort()); if (detailController) detailController.abort(); charts.forEach(chart => chart.dispose()); });
  refresh(true);
  const deepTask = new URLSearchParams(location.search).get('task_id'); if (deepTask) openDetail(deepTask);
})();
