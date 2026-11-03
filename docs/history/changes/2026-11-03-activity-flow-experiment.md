# 活动流程实验节点的本地草稿

## 背景

活动受众中的实验人群包只在流程入口分组，无法表达用户走到某一步后再按实验分支前进。用户确认由活动流程单独引用实验库定义，受众和各流程节点独立分组；本轮只完成浏览器可操作页面，待验收后再设计管理 API 与 Core 接入。

## 关键实现

- 活动详情增加实验节点列表、添加与编辑表单，支持在受众入口、投放项回执路径和已有实验分支上插入节点，并配置 A 组、B 组及对照组去向。列表和画布详情按最终合成图显示实际首跳与入边位置。
- 版本化浏览器草稿保存节点、连线与构造用户的模拟分组。受众入口与每个流程节点使用独立分组身份；实验比例和策略仍由实验库共享管理。
- 合成图拒绝循环、无效引用、不可达投放项和不可达实验节点。播放前复核管理 API 活动流程、实验库定义、受众目录与草稿快照；状态变化时提示刷新或修复，不沿旧图演练。
- 活动受众区只保留受众管理入口；活动实验区与流程画布明确标注浏览器草稿不会进入管理 API、Core 或真实执行。

## 关键文件

- `frontend/demos/operations-flow/flow-experiment-draft.mjs`：独立草稿格式、读取校验、模拟分组与存储失败提示。
- `frontend/demos/operations-flow/flow-experiment-graph.mjs`：合成路径、分支首跳、可达性校验及构造样本演练。
- `frontend/demos/operations-flow/flow-experiment-ui.mjs`：活动实验列表、详情编辑与保存前校验。
- `frontend/demos/operations-flow/activity-flow-canvas.mjs`：节点画布、实际入边和播放前新鲜度检查。
- `frontend/demos/operations-flow/activity-flow-experiment-play.test.mjs`、`flow-experiment-graph.test.mjs`：多节点路径、跨页变化、损坏草稿和孤立节点回归。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `node --test frontend/demos/operations-flow/*.test.mjs` | 退出码 0 | 当前产品提交的完整前端回归通过，日志 `/tmp/gmp-81-orphan-head-tests.log` 首行记录提交 SHA |
| `node frontend/demos/operations-flow/export.mjs /tmp/gmp-81-orphan-head-export.html` | 退出码 0 | 单文件离线页面生成，日志 `/tmp/gmp-81-orphan-head-export.log` 记录提交 SHA |
| `go test ./...`（`management`、`gmp-core`） | 退出码 0 | 管理服务与 Core 既有回归通过 |
| `node --check`、`python3 test/docs/consistency_check.py`、`git diff --check` | 退出码 0 | 修改模块语法、文档一致性与差异检查通过 |
| 本地浏览器 | 通过 | 桌面及 390px 创建、嵌套插入、编辑、刷新、演练与停用状态走查；孤立节点改线被拒绝保存且显示具体节点 |
| 独立只读评估器 | `PASS` | 审查产品提交 `26d1f02`，无阻断问题；评估器的临时目录权限限制和未独立复走浏览器已列为缺失验证 |
| GitHub `gates` | 成功 | 产品提交 `26d1f02` 的门禁通过 |

## 风险与未覆盖项

- 真风险（放行后续跟进）：跨标签页并发写入本地草稿没有原子保证；接入真实服务时须由服务端存储与版本化契约解决。浏览器草稿不代表生产染色或真实执行结果。
- 本次不覆盖项：管理 API/Core 的流程实验节点协议、生产染色、指标与真实执行；损坏缓存的页面提示和禁用演练由页面级自动化回归覆盖，未在浏览器手工破坏本地缓存。
- 外部系统读写：页面读取现有管理 API 活动数据，活动删除沿用现有 API 写入；新增节点只读写浏览器本地草稿，不向管理 API、Core 或生产服务写入实验节点。存储日志只记录组件、操作和结果。
- 回滚：撤销本轮前端提交可恢复旧活动页面；独立浏览器草稿键不会修改既有活动 API 数据。回退后保留的本地草稿不参与旧页面流程。
- 需要后续动作的事项：用户验收本地操作后，另立任务设计管理 API 与 Core 的节点持久化和稳定分组契约。
