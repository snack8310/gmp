# 活动流程实验节点实施计划

> **实施方式：** 当前会话顺序执行；以[已确认设计](../specs/2026-11-03-activity-flow-experiment-design.md)和[任务卡](../../ai-loop/task-cards/81-activity-flow-experiment.md)为准。任务关联 issue #81，分支 `feat/81-activity-flow-experiment`。

## 目标与边界

活动详情提供独立的实验节点编辑，引用实验库运行中的共享定义，插入活动流程有效路径，并在画布与本地构造演练中分组。保存仅写独立浏览器草稿键，不改变管理 API、Core、现有受众缓存格式或真实执行。高风险确认已由用户在本会话完成。

## 任务 1：本地草稿与分组

- 新增 `frontend/demos/operations-flow/flow-experiment-draft.mjs` 和对应测试。
- 定义版本化草稿、严格读取、原文保留、写失败告警、结构化读写日志；仅接受构造 UID 染色。
- 节点分配键纳入实验、活动和节点 ID，复用 `assignGroup`；预览不写，首次模拟仅在显式演练时写。
- 先写损坏缓存、写失败、隔离分组、比例变更的失败测试，再实现并运行测试。

## 任务 2：合成图和路径验证

- 新增 `frontend/demos/operations-flow/flow-experiment-graph.mjs` 和对应测试。
- 基于 `buildActivityGraph` 的连接构建可插入点：入口、已保存回执边、未接续回执出口、已有实验分支、空流程普通受众入口；排除受众包对照结束路径。
- 每节点三组必须有明确目标。验证引用运行态、锚点仍存在、目标存在、无环、原 API 投放项不因改线不可达。失效草稿保留并显示修复原因。
- 先写入口、回执、嵌套、失效及不可达测试，再实现并运行测试。

## 任务 3：页面、画布和演练

- 在 `campaign-ui.mjs` 增加独立“活动实验”容器；在 `campaign-controller.mjs` 挂载编辑器；`app.mjs` 建立独立草稿仓库并传递实验库定义。
- 在 `activity-flow-canvas.mjs` 按合成图渲染实验节点及 A/B/对照分支；按同一图进行演练，仅点击播放时记录首次模拟染色，展示每节点独立人数与到达次数。
- 实验库删除时通过 `experiment-ui.mjs` 检查流程草稿引用；停用与跨标签页缺失引用按无效处理。
- 页面和画布遵守现有标准间距、控件和移动端排版，按钮可键盘操作。
- 扩充 UI 与画布测试，先观察失败再实现；保持旧 API 编辑路径回归。

## 任务 4：导出、说明与验收

- 更新 `export.mjs`、`serve.mjs`、前端 README，使离线文件包含新模块和样式。
- 执行 `node --test frontend/demos/operations-flow/*.test.mjs`、`node --check`、离线导出、文档门禁、`git diff --check` 和桌面/390px 浏览器走查。
- 独立只读评估器基于任务卡、diff、执行证据作 `PASS`/`FAIL`/`NEEDS-EVIDENCE` 裁决。通过后补 `docs/history/changes/` 事实记录，再交人工审核。当前 issue #80 的既有阻断不属于本 issue 的通过证据。

## 评估重点

流程图与演练是否同路；受众与节点是否互不染色；同 UID 多路到达是否只计一次；API 改线、实验停用/删除、缓存损坏是否失败关闭；对照臂是否保持无投放；页面是否把本地草稿误写为 API 已保存。
