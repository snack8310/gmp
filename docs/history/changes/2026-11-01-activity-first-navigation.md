# 变更证据：活动承载投放项与共享资产入口

关联 issue #74；分支 `feat/74-activity-first-navigation`；现有 PR #75。此证据记录当前最新澄清后的实现。前一版评估通过的 SHA `ce63d74bd61dc583fdeee5d43c337f2e6fc07d85` 仍有活动内「组合」页签，已被用户后续澄清取代，不能作为本版验收结论。

## 背景与范围

最外层提供「活动、受众、实验」三个一级入口；受众、实验与实验人群包继续是可跨活动复用的共享资产。用户进一步澄清：活动本身代表一组能力，投放项是活动内容清单，不需要另设「组合」对象或组合页。当前实现以活动详情直接承载活动信息、投放项列表及已有活动流程边关系。活动和投放项继续由本机管理 API 管理；共享资产继续使用浏览器本地存储。本次不改服务端契约、核心领域模型、活动流程边语义、持久化格式或核心 `style.css`。

## 关键实现

- 保留全局「活动、受众、实验」一级导航；选择活动后直接进入活动详情。
- 移除活动内页签、组合沙盘的 HTML、启动逻辑、模型模块及其专属测试，以及受众编辑驱动沙盘的联动；静态服务器白名单不再暴露该模块。旧 `compose` 深链映射到活动详情，旧 `#compose` 回到活动列表。活动流程边只在活动详情只读展示。
- 活动详情继续展示可编辑的活动信息、投放项清单、共享资产入口和现有活动流程关系。投放项仍从管理 API 读取和写入。
- 受众、实验与实验人群包保持原共享身份和存储键；活动与投放项表单继续通过统一人群目录引用，不新增活动到实验定义的直接关系。
- 活动侧栏样式仍在独立 `activity-navigation.css`；核心 `style.css` 保持未修改。离线导出不再打包独立沙盘模型模块。
- 路由版本和控制器版本保护仍阻止活动切换后过期读取、表单与删除操作覆盖当前活动。

## 关键文件

- `frontend/demos/operations-flow/index.html`、`app.mjs`：全局导航、活动详情入口与移除沙盘运行代码。
- `frontend/demos/operations-flow/activity-routing.mjs`、`activity-navigation.mjs`、`activity-navigation.css`：活动深链、侧栏和独立导航样式。
- `frontend/demos/operations-flow/campaign-ui.mjs`、`campaign-controller.mjs`：活动信息、投放项清单、流程边关系以及活动 CRUD。
- `frontend/demos/operations-flow/audience-ui.mjs`、`experiment-ui.mjs`：共享资产管理和活动入口文案。
- `frontend/demos/operations-flow/export.mjs`、`app.test.mjs`、`export.test.mjs`：离线导出和回归。
- `frontend/demos/operations-flow/README.md`、`docs/ai-loop/task-cards/74-activity-first-navigation.md`：当前产品边界、操作与验证范围。

## 验证结果

以下命令已在提交 `dcc9cacf391393358c5bb3f485c09c27cabe3158` 上实际执行。记录评估结论后生成的最终提交会再按固定 SHA 复核；PR #75 正文记录最终裁决。

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `node --test --test-reporter=dot frontend/demos/operations-flow/*.test.mjs` | 通过 | 前端回归通过；覆盖管理界面路由、共享资产、活动投放项、旧组合路由回退及离线导出检查。 |
| `find frontend/demos/operations-flow -maxdepth 1 -name '*.mjs' -exec node --check {} +` | 通过 | Demo 目录所有模块语法检查无诊断。 |
| `python3 test/docs/consistency_check.py` 与 `git diff --check` | 通过 | 文档一致性门禁通过，差异无空白错误。 |
| 离线 HTML 导出与内容检查 | 通过 | 导出成功；确认保留活动详情和投放项清单，无独立沙盘 section、播放控件、模拟运行时代码或模型模块。 |
| 本机 HTTP 静态服务 | 通过 | 首页返回 200；已删除的 `model.mjs` 返回 404。 |
| 浏览器桌面、窄屏与键盘走查 | 通过 | 桌面和 390×844 窄屏下确认全局一级菜单、活动列表及活动内容；活动详情无组合入口；首个菜单项可见 `:focus-visible` 与 2px 焦点轮廓。 |
| 资产库返回及浏览器历史 | 通过 | 活动详情→受众库的返回链接，以及浏览器后退/前进均回到预期页面。 |
| 独立只读评估 | PASS | 固定 SHA `dcc9cacf391393358c5bb3f485c09c27cabe3158` 无阻断问题；最终候选 SHA 变化后重新复核。此前含组合页签的 SHA `ce63d74bd61dc583fdeee5d43c337f2e6fc07d85` 的 PASS 不可沿用。 |

## 风险与未覆盖项

- 通用高风险标准 3 命中：活动路由与 CRUD 异步切换顺序。用户确认按高风险继续，但确认晚于首次实现提交；该时间顺序偏差仍按事实记录，不得写成事前确认。
- 管理 API 与浏览器/前端以外的生产环境未验证；演示不连接生产服务，不执行真实触达。
- Safari、Firefox 和极窄屏设备未覆盖；本次浏览器走查范围以实测结果为准。
- 旧组合沙盘模型和专属测试文件已删除；当前管理界面保留活动流程边关系的只读展示，不运行独立模拟。
- 无新增持久化迁移，无 API、活动流程边、人群圈选或实验分桶语义变更。
- 回滚：回退本分支前端和文档提交可恢复此前导航；未更改管理 API、领域模型或共享存储。
