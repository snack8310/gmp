# 实验详情与共用尺寸

- 关联：issue #80；分支 `feat/80-experiment-list-detail`。
- 背景：用户指出实验详情与活动等页面的样式不同，并要求抽取一组共用尺寸。
- 实现：`frontend/demos/operations-flow/style.css` 集中定义页头、面板、按钮、输入框及窄屏尺寸；活动和受众既有构件与实验详情共用这些值。`experiment-ui.mjs` 移除重复标题卡，将运行态和操作放入页头，详情改为整行面板；新建页复用相同面板。`experiment-page.css` 只保留实验自身的表格、字段与响应式布局。取值记录在 `docs/product/design-system.md`。
- 验证：`node --test frontend/demos/operations-flow/*.test.mjs` 为 84/84 通过，含离线导出；`node --check frontend/demos/operations-flow/experiment-ui.mjs`、`python3 test/docs/consistency_check.py`（10 项）、`git diff --check` 均通过。桌面和 390px 浏览器截图核对活动详情、受众编辑、实验列表/详情/新建；窄屏页面无整体横向溢出，受众和实验表单输入/下拉框为 38px、按钮为 34px。
- 评估：独立只读评估器对本轮视觉增量裁决 `PASS`；前两轮指出的按钮、页标题及受众表单固定高度已接入共用变量。评估器未在只读环境重跑需创建临时文件的导出用例，采纳主 Agent 的真实命令结果。
- 风险与未覆盖：共享 CSS 影响活动、受众、实验；已用对应页面截图与前端回归验证。历史侧栏、演练控件的专属尺寸未逐项重构。本轮不改领域语义、缓存、管理 API 或 Core；无新增外部系统读写。回滚本次样式与页面标记即可恢复旧版外观。
- issue #80 整体仍为 `FAIL`：重复染色记录 JSON 键未失败关闭，活动演练尚未明确说明其与当前实验分桶独立。视觉增量的 `PASS` 不解除这两项，不请求人工合并。
