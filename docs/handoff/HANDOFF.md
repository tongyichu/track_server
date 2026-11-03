# 开发交接：AGENTS 精简（2026-11-04）

## 请求与范围

- 用户在克隆 android-track-bakcup3 和 track-server-bakcup3 后，确认将偏大的 AGENTS 精简为关键规则与按需文档索引。
- 仅修改这两个副本的 Markdown 规则/参考文档；未改业务源码，未提交、推送、部署、构建或运行功能测试。
- 原交接完整归档为 [历史交接](history_handoff/HANDOFF-20261104-003606.md)，需恢复旧业务任务时读取。

## 已完成

- 根与 scoped AGENTS 保留默认验证、授权、风险边界与常用入口；详细业务/接口/UI 说明迁到 docs 参考文件，相关改动前按主题读取。
- 本仓 AGENTS 总体积 139,701 → 11,860 字节，减少 91.5%。
- 新增参考：docs/admin-routes.md, docs/admin-security.md, docs/admin-ui.md, docs/admin-workflows.md, docs/server-contracts.md, docs/server-flows.md。
- 修正迁移后的形象技能相对链接、陈旧的 Android uploader 路径；明确业务私有字段限制不影响后台授权查询，业务查询不外呼也不禁止后台人工专用接口。
- 按 agents-md-generator 技能进行了独立只读审查，审查指出的链接与范围问题已修正；采用 internal-docs 的中文、目录和代码围栏规范。

## 验证与剩余问题

- git diff --check、所有新增/修改规则与参考文档的本地 Markdown 链接/锚点和代码围栏检查通过；四份 AGENTS 均小于 16 KB。
- verify_agents.py 已执行：AGENTS 本身无错误或路径告警；全仓校验仍有 10 个既有超 1000 行源码文件问题，本次未修改业务源码。
- 全局 ~/.codex/AGENTS.md 为空，全局入口规则未落盘；这是已有环境状态，不属于本次两个仓库的精简范围。修复命令为 python ~/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write，本次未执行。
- 纯规则和文档维护无需构建；未新增或运行单元/UI/集成测试，未操作设备或生产。

## 后续维护

- 继续保持根文件精简；新增详细功能说明写对应参考/API 文档，仅在 AGENTS 更新入口或不变量。
- 用户随后明确授权“两个都推送代码”：将本次规则精简、参考文档和交接提交推送到本仓 origin/main；不发布 App、不部署 Server。最终提交与远端核验结果见本次会话回复及 Git 历史。
