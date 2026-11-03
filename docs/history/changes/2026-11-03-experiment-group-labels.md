# 实验配置页分组文案

- 关联：issue #80；分支 `feat/80-experiment-list-detail`。
- 背景：用户指出“A 臂 %”等比例标签像英文直译。
- 改动：`experiment-ui.mjs` 的新建、详情、列表、预览及模拟分配反馈改用“A 组”“B 组”“对照组”；比例字段标为“组占比 (%)”。字段 ID、比例算法、本地缓存、已发布人群包名称、管理 API 和 Core 均未修改。
- 验证：前端 `node --test frontend/demos/operations-flow/*.test.mjs` 为 84/84 通过；`node --check frontend/demos/operations-flow/experiment-ui.mjs`、文档一致性 10 项和 `git diff --check` 均通过。浏览器强制刷新后，新建和详情页的三组字段标签正确，实验页可见文案不再含“臂”。
- 评估：独立只读评估器对本轮文案增量裁决 `PASS`；全量回归和浏览器检查采用主 Agent 的真实执行结果，评估器独立重跑了定向测试及静态检查。
- 风险与未覆盖：仅展示文案变更，无新增外部系统读写。活动与受众中的既有“实验臂”技术资产及存量名称未改；如需统一其领域命名，应单独评估数据兼容。回滚本次文案改动即可恢复原显示。issue #80 原有两项整体阻断仍未解除。
