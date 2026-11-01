(function () {
  'use strict';
  // Describe only facts available in historical method/path records.
  const rules = [
    ['POST', 'accounts', '创建后台账号', '新后台账号'],
    ['POST', 'accounts/:account', '创建后台账号', '后台账号 {account}'],
    ['PUT', 'accounts/:account', '更新后台账号设置', '后台账号 {account}', '可能涉及名称、身份、启停状态或页面权限；历史记录未保存具体字段变化。'],
    ['POST', 'accounts/:account/reset-password', '重置后台账号密码', '后台账号 {account}', '密码内容不会记录在操作日志中。'],
    ['POST', 'password', '修改本人密码', '后台账号 {actor}', '密码内容不会记录在操作日志中。'],
    ['POST', 'security/email/current-code', '申请原邮箱验证码', '后台账号 {actor}', '用于验证原邮箱以更换找回邮箱，不代表邮箱已更换。'],
    ['POST', 'security/email/send-code', '申请新邮箱验证码', '后台账号 {actor}', '仅发送验证邮件，验证通过后才会保存邮箱。'],
    ['POST', 'security/email/verify', '验证并保存本人找回邮箱', '后台账号 {actor}', '验证成功会退出全部旧登录，管理员不能代改他人邮箱。'],
    ['POST', 'password-recovery/complete', '通过邮箱重新设置密码', '后台账号 {actor}', '这是邮箱找回流程的请求，成功表示验证通过并设置新密码；失败不代表由该账号本人发起。'],
    ['POST', 'releases', '发布或更新 App 版本', 'App 版本记录', '历史记录未保存本次版本号。'],
    ['DELETE', 'releases/:release', '删除 App 版本记录', '版本记录 {release}'],
    ['GET', 'releases/upload-token', '申请安装包上传凭证', '安装包上传', '申请凭证不代表安装包已经上传或发布。'],
    ['POST', 'releases/upload-package', '上传安装包', 'App 安装包', '上传成功不代表已发布版本。'],
    ['POST', 'scenic-spots/provider-content/backfill', '同步景点供应商资料', '景点目录', '景点页会自动分批触发此请求，连续记录不一定是手动重复操作；每条仅代表一批请求，具体同步结果请查看景点页。'],
    ['POST', 'scenic-spots/description/generate', '生成景点简介草稿', '景点简介', '生成草稿不代表简介已经保存。'],
    ['POST', 'scenic-spots', '创建景点', '新景点'],
    ['POST', 'scenic-spots/backfill', '回填景点目录', '景点目录', '批量处理的具体结果请查看景点页。'],
    ['PUT', 'scenic-spots/:spot', '修改景点资料', '景点 {spot}'],
    ['POST', 'scenic-spots/:spot/status', '调整景点发布状态', '景点 {spot}', '历史记录未保存本次发布、下线或归档的具体状态。'],
    ['POST', 'scenic-spots/:spot/provider-sources/verify', '验证景点供应商来源', '景点 {spot}'],
    ['POST', 'scenic-spots/:spot/provider-sources/preview', '预览景点供应商资料', '景点 {spot}', '预览不代表资料已经保存。'],
    ['POST', 'scenic-spots/:spot/photos', '上传景点图片', '景点 {spot}'],
    ['POST', 'scenic-spots/:spot/tracks', '为景点关联轨迹', '景点 {spot}', '历史记录未保存关联的轨迹编号。'],
    ['DELETE', 'scenic-spots/:spot/tracks/:track', '解除景点与轨迹的关联', '景点 {spot} · 轨迹 {track}'],
    ['POST', 'scenic-spots/:spot/merge', '合并景点', '景点 {spot}', '历史记录未保存另一方的景点编号。'],
    ['POST', 'scenic-spots/:spot/split', '拆分景点', '景点 {spot}'],
    ['POST', 'place-candidates/:track/confirm', '确认轨迹的景点候选', '轨迹 {track}', '历史记录未保存选中的候选景点。'],
    ['POST', 'place-candidates/:track/dismiss', '忽略轨迹的景点候选', '轨迹 {track}'],
    ['PUT', 'users/:user', '修改用户资料', '用户 {user}'],
    ['PUT', 'users/:user/internal', '调整用户内部人员标识', '用户 {user}'],
    ['POST', 'users/:user/restrictions', '设置用户账号限制', '用户 {user}'],
    ['DELETE', 'users/:user/restrictions/current', '解除用户账号限制', '用户 {user}'],
    ['PUT', 'tracks/:track', '修改轨迹资料', '轨迹 {track}'],
    ['DELETE', 'tracks/:track', '删除轨迹', '轨迹 {track}'],
    ['POST', 'track-submissions/:submission/review', '审核轨迹投稿', '投稿 {submission}', '历史记录未保存本次是通过还是驳回；请在投稿详情查看审核结果。'],
    ['PUT', 'route-groups/:group/name', '修改路线组名称', '路线组 {group}'],
    ['POST', 'route-groups/:group/merge', '合并路线组', '路线组 {group}', '历史记录未保存另一方的路线组编号。'],
    ['DELETE', 'route-groups/:group/members/:track', '从路线组移除轨迹', '路线组 {group} · 轨迹 {track}'],
    ['PUT', 'route-groups/:group/representative', '调整路线组代表轨迹', '路线组 {group}', '历史记录未保存选中的代表轨迹编号。'],
    ['PUT', 'route-groups/:group/introduction', '保存路线介绍', '路线组 {group}'],
    ['POST', 'route-groups/:group/introduction/publish', '发布路线介绍', '路线组 {group}'],
    ['POST', 'route-groups/:group/introduction/unpublish', '下线路线介绍', '路线组 {group}'],
    ['PUT', 'feedback-correction-config', '更新纠错选项配置', '反馈与纠错'],
    ['PUT', 'feedbacks/:feedback/status', '处理反馈或纠错', '反馈 {feedback}', '历史记录未保存具体处理状态和回复内容；请在反馈详情查看。']
  ].map(function (rule) {
    const keys = [], pattern = rule[1].split('/').map(function (part) {
      if (part.startsWith(':')) { keys.push(part.slice(1)); return '([^/]+)'; }
      return part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }).join('/');
    return { method: rule[0], pattern: new RegExp('^' + pattern + '$'), keys, action: rule[2], target: rule[3], note: rule[4] || '' };
  });

  function decode(value) { try { return decodeURIComponent(value); } catch (_) { return value; } }
  function describe(entry) {
    const method = String(entry.method || '').toUpperCase(), path = String(entry.path || '');
    if (path.startsWith('/admin/api/')) {
      const route = path.slice('/admin/api/'.length);
      for (const rule of rules) {
        if (rule.method !== method) continue;
        const match = rule.pattern.exec(route);
        if (!match) continue;
        const values = { actor: entry.username || '未知账号' };
        rule.keys.forEach((key, index) => { values[key] = decode(match[index + 1]); });
        return { action: rule.action, target: rule.target.replace(/\{(\w+)\}/g, (_, key) => values[key]), note: rule.note };
      }
    }
    return { action: '执行后台操作', target: '后台管理', note: '该操作暂未配置中文说明，可展开请求详情查看原始记录。' };
  }

  function outcome(entry) {
    const status = Number(entry.status || 0);
    if (!status) return { label: '结果待确认', color: 'bg-yellow-lt text-yellow', note: '已记录操作发起，但没有最终响应记录；可能仍在处理或结果记录未写入，不能据此认定成功或失败。' };
    if (status >= 200 && status < 300) return { label: '成功', color: 'bg-green-lt text-green', note: '请求成功返回；批量任务的逐项处理结果需在对应页面查看。' };
    if (status >= 300 && status < 400) return { label: '已跳转，结果待确认', color: 'bg-yellow-lt text-yellow', note: '请求返回了跳转响应，不能据此认定操作成功。' };
    const reasons = { 400: '请求参数或内容不符合要求。', 401: '登录已失效，需要重新登录。', 403: '权限不足或请求来源校验未通过。', 404: '操作对象或接口不存在。', 409: '资料已变化或操作发生冲突，请刷新后核对。', 413: '上传文件超过大小限制。', 422: '提交的资料未通过校验。', 429: '请求过于频繁，请稍后重试。' };
    return { label: '请求失败', color: 'bg-red-lt text-red', note: reasons[status] || (status >= 500 ? '服务处理异常；请先核对实际变更结果，再决定是否重试。' : '请求未成功完成，可结合 HTTP 状态排查。') };
  }

  function visibleEntries(items) {
    const baseID = item => String(item.id || '').replace(/:(attempt|result)$/, '');
    const completed = new Set(items.filter(item => String(item.id || '').endsWith(':result')).map(baseID));
    return items.filter(item => !String(item.id || '').endsWith(':attempt') || !completed.has(baseID(item)));
  }
  window.AdminAuditLog = { describe, outcome, visibleEntries };
})();
