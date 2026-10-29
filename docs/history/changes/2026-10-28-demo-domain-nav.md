# 变更证据：运营流程 Demo 按领域组织

关联：issue #57，PR #58。

## 背景

原页面按「人群管理 → 运营沙盘」组织入口，实验管理散落在受众编辑和沙盘中，容易把演示顺序误读为 `audience`、`experimentation`、`campaign` 的职责划分。本次把各域对象放回各自页面，跨域引用和播放集中到最后的组合页。

## 关键实现

- 顶层按受众、实验、运营、组合排列。宽屏为左侧导航，窄屏为顶栏；哈希可深链，当前项使用 `aria-current="page"`，离开组合页会暂停播放。
- 受众保留人群创建、编辑、预览，补上删除；实验引用只从已保存实验中选择。实验页沿用原缓存格式管理实验，展示固定 45/45/10 示意、稳定分配及染色和放量的只读说明。删除仍被人群引用的实验会被拒绝。
- 运营页仅说明活动多臂、投放项一动作和活动流程边，标出 #44/#55 关系；沙盘、播放、流程实验节点、统计和场景留在组合页。删除当前组合引用的人群后，引用会切换到可用人群或清空。
- 离线 HTML 导出包含新增的实验界面模块。页面继续使用构造数据，不连接 `gmp-core` 或真实触达服务。

## 关键文件

- `frontend/demos/operations-flow/index.html`、`style.css`、`app.mjs`：受众、实验、运营与组合页面，响应式导航及路由。
- `frontend/demos/operations-flow/experiment-ui.mjs`、`audience-ui.mjs`、`audience.mjs`：实验管理、人群引用与删除边界。
- `frontend/demos/operations-flow/export.mjs`：离线页面模块打包。
- `frontend/demos/operations-flow/*.test.mjs`：导航、CRUD、引用、缓存、播放与导出回归。
- `frontend/demos/operations-flow/README.md`、`docs/ai-loop/task-cards/57-demo-domain-nav.md`：按领域说明操作和任务入口检查。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `node --test frontend/demos/operations-flow/*.test.mjs` | 退出码 0 | 覆盖旧缓存、被引用实验删除保护、组合引用清理与离线导出 |
| 改动 JS 模块及测试逐个 `node --check` | 退出码 0 | 语法检查无诊断 |
| `python3 test/docs/consistency_check.py` | 退出码 0 | 文档一致性门禁 |
| `python3 test/docs/consistency_check_test.py` | 退出码 0 | 覆盖空断言、事实引用、编号、文档归类、设计文档结构、色板与对比度、相对链接的变异 |
| `git diff --check` | 退出码 0 | 无空白错误 |
| Chrome 页面走查 | 部分通过 | 实际页面完成实验创建与删除；独立评估器核对 1280px 和约 500px 截图 |
| PR head 的 CI | 通过 | 以 PR checks 关联的当前 head 为准，不在版本化文档固定 run ID |

## 风险与未覆盖项

- 真风险（仅观察）：精确 390px 视口、键盘焦点和跨浏览器行为尚未验证；约 500px 的窄屏布局已走查。
- 本次不覆盖项：生产服务、真实渠道、实验指标界面、可编活动流程图，以及 #55/#45 的领域裁定。
- 外部系统影响：浏览器只读写原演示用 localStorage 命名空间；GitHub 写入 issue、分支和 PR。无数据库、缓存服务、消息队列或渠道改动。
- 回滚：撤回本 PR 即可；无数据迁移，原本地缓存结构及版本保持兼容。
- 需要后续动作的事项：人工最终审核 PR；如要对特定设备或浏览器承诺完整可用性，补精确视口与键盘检查。
