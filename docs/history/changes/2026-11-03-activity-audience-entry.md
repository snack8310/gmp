# 活动受众区只引导至受众库

- 关联：issue #80；分支 `feat/80-experiment-list-detail`。
- 背景：用户在活动明细的“活动受众”区指出，实验库的新建与管理入口不属于这里。
- 改动：`campaign-ui.mjs` 移除活动详情受众区的实验库按钮；`campaign-controller.mjs` 移除活动编辑受众选择区的同类按钮。两处继续使用原受众库链接和返回活动的路由参数。`frontend/demos/operations-flow/README.md` 更新当前入口说明。普通人群组、已发布实验人群包的选择能力和全局实验导航未修改。
- 验证：前端 `node --test frontend/demos/operations-flow/*.test.mjs`、离线导出、改动模块的 `node --check`、文档一致性检查和 `git diff --check` 均通过。浏览器中活动详情及编辑页的受众操作区只保留受众库链接，地址保留 `return=activities%2Fcampaign-branch%2Fcampaign`；全局实验入口仍存在。
- 评估：独立只读评估器对本轮小任务裁决 `PASS`，独立复跑相关前端测试通过；全量回归与浏览器编辑页检查采用主 Agent 的真实执行结果并交叉核对实现。
- 风险与未覆盖：旧 issue #74 的任务卡曾要求活动上下文直达实验库，本次用户明确纠正了活动受众区的职责；历史任务卡不追溯改写。本轮未改变活动受众、实验分流、浏览器缓存、API 或 Core，也无新增外部系统读写。回滚这两处导航标记即可恢复旧入口。issue #80 的原有两项整体阻断仍未解除。
