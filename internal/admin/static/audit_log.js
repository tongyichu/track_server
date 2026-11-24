(function () {
  'use strict';
  // Describe only facts available in historical method/path records.
  const rules = [
    ['POST', 'announcements', '创建通知', '通知 · 未记录编号', '是否已发布以通知状态及发布时间为准。'],
    ['PUT', 'announcements/:notice', '更新通知', '通知 {notice}', '可能涉及编辑、定时安排、取消定时、发布或撤回；具体状态与发布轮次以通知详情为准。'],
    ['POST', 'ai/tasks/:task/cancel', '请求中断', 'AI 任务 {task}', '请求成功仅代表已登记中断；最终结果以 AI 使用页为准，已发生的消耗不会撤回。'],
    ['POST', 'ai/tasks/:task/retry', '重试任务', 'AI 任务 {task}', '重新读取当前资料，可能产生新的消耗；完成情况见关联重试任务。'],
    ['PUT', 'ai/policies/:feature/quota', '调整请求上限', 'AI 功能 {feature}', '只调整请求上限，0表示不限；不清空用量，不修改暂停或并发配置。'],
    ['PUT', 'ai/policies/:feature', '更新 AI 配置', 'AI 功能 {feature}'],
    ['PUT', 'ai/prices', '更新模型价格', 'AI 费用估算', '仅影响后续请求的费用估算，不修改历史记录或供应商账单。'],
    ['POST', 'tracks/:track/labels/generate', '生成图案建议', '轨迹 {track}', '生成建议不代表已保存。'],
    ['POST', 'tracks/:track/description/generate', '生成说明建议', '轨迹 {track}', '生成建议不代表已保存。'],
    ['PUT', 'tracks/:track/description', '保存路线说明', '轨迹 {track}', '只保存说明，不提交详情里的其他草稿。'],
    ['POST', 'accounts', '创建账号', '后台账号 · 未记录登录名'],
    ['POST', 'accounts/:account', '创建账号', '后台账号 {account}'],
    ['PUT', 'accounts/:account', '更新账号', '后台账号 {account}', '可能涉及名称、身份、启停状态或页面权限；历史记录未保存具体字段变化。'],
    ['POST', 'accounts/:account/reset-password', '重置密码', '后台账号 {account}', '密码内容不会记录在操作日志中。'],
    ['POST', 'password', '修改本人密码', '后台账号 {actor}', '密码内容不会记录在操作日志中。'],
    ['POST', 'security/email/current-code', '发送原邮箱验证码', '后台账号 {actor}', '用于验证原邮箱以更换找回邮箱，不代表邮箱已更换。'],
    ['POST', 'security/email/send-code', '发送新邮箱验证码', '后台账号 {actor}', '仅发送验证邮件，验证通过后才会保存邮箱。'],
    ['POST', 'security/email/verify', '验证找回邮箱', '后台账号 {actor}', '验证成功会退出全部旧登录，管理员不能代改他人邮箱。'],
    ['POST', 'password-recovery/complete', '找回密码', '后台账号 {actor}', '这是邮箱找回流程的请求，成功表示验证通过并设置新密码；失败不代表由该账号本人发起。'],
    ['POST', 'music', '添加背景音乐', '乐库'],
    ['PUT', 'music/layout', '调整乐库', '乐库'],
    ['DELETE', 'music/:music', '删除音乐', '音乐 {music}'],
    ['PUT', 'music/:music', '更新音乐', '音乐 {music}'],
    ['POST', 'releases', '保存发布版本', 'App 版本 · 未记录版本号'],
    ['PUT', 'releases/:version/policy', '保存升级策略', 'Android 构建 {version}'],
    ['DELETE', 'releases/:release', '删除发布版本', '版本记录 {release}'],
    ['GET', 'releases/upload-token', '申请上传凭证', '安装包上传', '申请凭证不代表安装包已经上传或发布。'],
    ['POST', 'releases/upload-package', '上传安装包', 'App 安装包', '上传成功不代表已发布版本。'],
    ['POST', 'scenic-spots/provider-content/backfill', '同步景点资料', '景点目录', '每条记录对应一批同步请求。当前景点页需确认后才开始同步，旧版本进入页面时可能触发同步。'],
    ['POST', 'scenic-spots/description/generate', '生成简介草稿', '景点简介', '生成草稿不代表简介已经保存。'],
    ['POST', 'scenic-spots', '创建景点', '景点 · 未记录编号'],
    ['POST', 'scenic-spots/backfill', '回填景点目录', '景点目录', '批量处理的具体结果请查看景点页。'],
    ['PUT', 'scenic-spots/:spot', '更新景点', '景点 {spot}'],
    ['POST', 'scenic-spots/:spot/status', '调整景点状态', '景点 {spot}', '历史记录未保存本次调整的具体状态。'],
    ['POST', 'scenic-spots/:spot/provider-sources/verify', '验证景点来源', '景点 {spot}'],
    ['POST', 'scenic-spots/:spot/provider-sources/preview', '预览景点资料', '景点 {spot}', '预览不代表资料已经保存。'],
    ['POST', 'scenic-spots/:spot/photos', '上传景点图片', '景点 {spot}'],
    ['PUT', 'scenic-spots/:spot/photos', '保存景点图片', '景点 {spot}', '仅保存图片关联、顺序与说明，不提交未保存的文字资料。'],
    ['PUT', 'regions/:node/photos', '保存宣传图片', '地区 {node}', '仅保存图片关联、顺序与说明，不提交未保存的文字资料。'],
    ['POST', 'regions/:node/photos', '上传宣传图片', '地区 {node}'],
    ['PUT', 'regions/catalog', '更新地区目录', '地区目录'],
    ['PUT', 'regions/node', '更新地区', '地区 · 未记录编号'],
    ['DELETE', 'regions/:node', '删除国家', '地区 {node}'],
    ['POST', 'regions/photos/pixabay/search', '查询候选图片', '省市宣传图片', '人工查询只返回候选图片，不自动下载或修改地区目录。'],
    ['POST', 'regions/:node/photos/pixabay/import', '导入候选图片', '地区 {node}', '暂存图片资源；仍需保存当前节点才关联到地区目录。'],
    ['POST', 'scenic-spots/:spot/tracks', '关联轨迹', '景点 {spot}', '历史记录未保存关联的轨迹编号。'],
    ['DELETE', 'scenic-spots/:spot/tracks/:track', '解除关联', '景点 {spot} · 轨迹 {track}'],
    ['POST', 'scenic-spots/:spot/merge', '合并景点', '景点 {spot}', '历史记录未保存另一方的景点编号。'],
    ['POST', 'scenic-spots/:spot/split', '拆分景点', '景点 {spot}'],
    ['POST', 'place-candidates/:track/confirm', '确认景点候选', '轨迹 {track}', '历史记录未保存选中的候选景点。'],
    ['POST', 'place-candidates/:track/dismiss', '忽略景点候选', '轨迹 {track}'],
    ['PUT', 'users/:user', '更新用户', '用户 {user}'],
    ['PUT', 'users/:user/internal', '更新内部标识', '用户 {user}'],
    ['POST', 'users/:user/restrictions', '设置账号限制', '用户 {user}'],
    ['DELETE', 'users/:user/restrictions/current', '解除账号限制', '用户 {user}'],
    ['PUT', 'tracks/:track', '更新轨迹', '轨迹 {track}'],
    ['DELETE', 'tracks/:track/previews', '删除预览图', '轨迹 {track}'],
    ['DELETE', 'tracks/:track', '删除轨迹', '轨迹 {track}'],
    ['POST', 'tracks/:track/restore', '恢复轨迹', '轨迹 {track}'],
    ['POST', 'tracks/:track/purge', '彻底删除轨迹', '轨迹 {track}'],
    ['POST', 'tracks/import', '导入轨迹', '批量轨迹'],
    ['POST', 'tracks/:track/place-refresh', '刷新景点候选', '轨迹 {track}'],
    ['POST', 'tracks/:track/scenic-spot', '设置关联景点', '轨迹 {track}'],
    ['POST', 'track-submissions/:submission/review', '审核投稿', '投稿 {submission}', '历史记录未保存本次是通过还是驳回；请在投稿详情查看审核结果。'],
    ['PUT', 'route-groups/:group/name', '修改组名', '路线组 {group}'],
    ['POST', 'route-groups/:group/merge', '合并路线组', '路线组 {group}', '历史记录未保存另一方的路线组编号。'],
    ['DELETE', 'route-groups/:group/members/:track', '移除组内轨迹', '路线组 {group} · 轨迹 {track}'],
    ['PUT', 'route-groups/:group/representative', '设置代表轨迹', '路线组 {group}', '历史记录未保存选中的代表轨迹编号。'],
    ['PUT', 'route-groups/:group/introduction', '保存路线介绍', '路线组 {group}'],
    ['POST', 'route-groups/:group/introduction/generate', '生成路线介绍草稿', '路线组 {group}'],
    ['POST', 'route-groups/:group/introduction/publish', '发布路线介绍', '路线组 {group}'],
    ['POST', 'route-groups/:group/introduction/unpublish', '下线路线介绍', '路线组 {group}'],
    ['PUT', 'feedback-correction-config', '更新纠错选项', '反馈与纠错'],
    ['PUT', 'feedbacks/:feedback/status', '处理反馈', '反馈 {feedback}', '历史记录未保存具体处理状态和回复内容；请在反馈详情查看。']
  ].map(function (rule) {
    const keys = [], pattern = rule[1].split('/').map(function (part) {
      if (part.startsWith(':')) { keys.push(part.slice(1)); return '([^/]+)'; }
      return part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }).join('/');
    return { method: rule[0], pattern: new RegExp('^' + pattern + '$'), keys, action: rule[2], target: rule[3], note: rule[4] || '' };
  });

  function decode(value) { try { return decodeURIComponent(value); } catch (_) { return value; } }
  function describe(entry, targetNames = {}) {
    const method = String(entry.method || '').toUpperCase(), path = String(entry.path || '');
    if (path.startsWith('/admin/api/')) {
      const route = path.slice('/admin/api/'.length);
      for (const rule of rules) {
        if (rule.method !== method) continue;
        const match = rule.pattern.exec(route);
        if (!match) continue;
        const values = { actor: entry.username || '未知账号' };
        rule.keys.forEach((key, index) => { values[key] = decode(match[index + 1]); });
        const target = rule.target.replace(/\{(\w+)\}/g, (_, key) => {
          const id = values[key];
          const names = targetNames[key === 'actor' ? 'account' : key];
          const name = names && Object.prototype.hasOwnProperty.call(names, id) ? String(names[id] || '').trim() : '';
          return name && name !== id ? `${name}（${id}）` : id;
        });
        return { action: rule.action, target, note: rule.note };
      }
    }
    return { action: '后台操作', target: '未识别对象', note: '可展开查看原始请求。' };
  }

  function outcome(entry) {
    const status = Number(entry.status || 0);
    if (!status) return { label: '待确认', color: 'bg-yellow-lt text-yellow', note: '已记录操作发起，但没有最终响应记录；可能仍在处理或结果记录未写入，不能据此认定成功或失败。' };
    if (status >= 200 && status < 300) return { label: '成功', color: 'bg-green-lt text-green', note: '请求成功返回；批量任务的逐项处理结果需在对应页面查看。' };
    if (status >= 300 && status < 400) return { label: '待确认', color: 'bg-yellow-lt text-yellow', note: '请求返回了跳转响应，不能据此认定操作成功。' };
    const reasons = { 400: '请求参数或内容不符合要求。', 401: '登录已失效，需要重新登录。', 403: '权限不足或请求来源校验未通过。', 404: '操作对象或接口不存在。', 409: '资料已变化或操作发生冲突，请刷新后核对。', 413: '上传文件超过大小限制。', 422: '提交的资料未通过校验。', 429: '请求过于频繁，请稍后重试。' };
    return { label: '失败', color: 'bg-red-lt text-red', note: reasons[status] || (status >= 500 ? '服务处理异常；请先核对实际变更结果，再决定是否重试。' : '请求未成功完成，可结合 HTTP 状态排查。') };
  }

  function visibleEntries(items) {
    const baseID = item => String(item.id || '').replace(/:(attempt|result)$/, '');
    const completed = new Set(items.filter(item => String(item.id || '').endsWith(':result')).map(baseID));
    return items.filter(item => !String(item.id || '').endsWith(':attempt') || !completed.has(baseID(item)));
  }
  window.AdminAuditLog = { describe, outcome, visibleEntries };
})();
