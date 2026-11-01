# 变更证据：活动、受众与实验的一级导航

关联 issue #74；分支 `feat/74-activity-first-navigation`。用户确认受众定义、实验定义和实验人群包继续作为跨活动共享资产。

## 背景

operations-flow Demo 原先将受众、实验、运营和组合放在同一层，活动列表藏在运营域内。现调整为全局「活动、受众、实验」三个一级入口；活动工作区内只保留「运营、组合」。受众定义、实验定义和实验人群包继续作为跨活动共享资产。活动和投放项仍使用管理 API 的稳定 ID，未改变服务端契约和共享资产存储。

## 关键实现

- 新增统一活动路由解析与导航模块，支持活动 ID 编码、深链、旧运营活动链接映射、未知活动提示及浏览器历史。
- 新增独立 `activity-navigation.css`；核心 `style.css` 未修改。桌面侧栏提供活动、受众、实验入口，并在活动上下文显示活动列表与「运营、组合」页签；窄屏改为可横向访问的导航条。
- 活动详情的「编辑活动」和「＋ 添加投放项」锚点复用核心 `style.css` 中既有的 `.primary-link` 样式；只补 class，不改核心样式表，锚点仍按链接导航。
- 受众与实验一级列表复用原共享存储，活动详情和投放项编辑提供资产库入口；组合继续引用当前模拟资产，不新增活动关联。浏览器本地人群定义不会自动同步到管理 API 的统一引用目录，投放项只能选择该目录当前开放的引用。
- 运营控制器根据路由活动 ID 读写活动与投放项。每次渲染生成版本号；切换路由时先使旧版本失效，旧表单提交、活动删除和投放项删除入口会拒绝过期事件。
- README、离线导出和服务端静态文件白名单同步支持新的导航结构。

## 关键文件

- `frontend/demos/operations-flow/index.html`：全局一级导航和活动内工作区结构。
- `frontend/demos/operations-flow/activity-routing.mjs`、`activity-navigation.mjs`：全局/活动路由及活动列表状态。
- `frontend/demos/operations-flow/activity-navigation.css`：独立导航样式；核心 `style.css` 保持不变。
- `frontend/demos/operations-flow/app.mjs`、`campaign-controller.mjs`、`campaign-ui.mjs`：活动上下文渲染和过期写控件保护。
- `frontend/demos/operations-flow/app.test.mjs`、`activity-routing.test.mjs`、`activity-navigation.test.mjs`、`campaign-activities.test.mjs`：活动切换、共享资产、深链、过期控件及活动操作锚点样式回归。
- `docs/ai-loop/task-cards/74-activity-first-navigation.md`：范围、验收、高风险和时间顺序记录。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `node --test frontend/demos/operations-flow/*.test.mjs` | 通过 | 前端回归无失败；覆盖路由、共享资产、CRUD、异步切换、过期提交/删除控件、活动操作锚点样式与离线导出。 |
| 对 `frontend/demos/operations-flow/*.mjs` 执行 `node --check` | 通过 | Demo 模块语法检查无诊断。 |
| `python3 test/docs/consistency_check.py` | 通过 | 文档一致性门禁通过；本次更正后重新执行。 |
| `node frontend/demos/operations-flow/export.mjs /tmp/gmp-activity-navigation-final.html` 与导出内容检查 | 通过 | 生成单文件；包含内联活动导航、核心 `.primary-link` 样式和控件版本校验逻辑。 |
| `git diff HEAD^ HEAD --check` 与范围检查 | 通过 | 无空白问题；未修改核心 `style.css`、`management/` 或 `gmp-core/`。 |
| 主 Agent CUA 桌面、窄屏与活动详情预览 | 通过 | 全局活动/受众/实验入口、活动内运营/组合、窄屏布局及按钮式链接可见。 |
| 主 Agent CUA 键盘焦点 | 通过 | Tab 顺序依次到达活动、受众、实验、新建活动和活动链接；每项均显示 `2px solid` 焦点轮廓。 |
| 主 Agent CUA 浏览器历史 | 通过 | 从活动详情打开受众库后，后退回活动详情、前进回受众库；返回链接指向原活动详情。 |
| 独立只读评估器第 1 轮 | `FAIL` | 固定 SHA `41acbb984c9583005d80018ffaeacd57384e845c`；阻断项为记录仍写旧的活动内四页签、旧 SHA 的 PASS 被误写为当前结论，以及缺少键盘焦点实测证据。后续提交已更正记录并补充键盘走查。 |
| 独立只读评估器第 2 轮 | `NEEDS-EVIDENCE` | 固定 SHA `1d600820e581abe8c8e5d800ffd5a8c08b6b220a`；无阻断代码问题，但要求全量前端回归、模块语法和离线导出检查绑定送审 SHA。上述命令已在该固定 SHA 上重跑通过；最终裁决以 PR 中针对最终 SHA 的结果为准。 |

## 风险与未覆盖项

- 高风险标准 3 命中：活动路由与 CRUD 事件的并发/执行顺序控制。用户于 2026-11-01 确认按高风险继续。确认晚于首个实现提交；该流程偏差未伪记为事前确认。
- 真实 Go 管理 API CRUD / HTTP 集成未运行：本机 Go 工具链版本低于项目要求，下载要求版本失败。浏览器预览使用本地只读模拟 API；不将其当作真实 API 验证。
- 第 1 轮独立评估器未重放真实浏览器走查；评估器运行环境无法完成写临时目录的导出测试。主 Agent 提供的全量前端测试、导出和浏览器走查结果作为证据审查。后续评估结论以 PR 中针对最终固定 SHA 的裁决为准。
- Safari、Firefox 与极窄屏设备未覆盖。程序化导航设置哈希后也主动渲染，可能重复发起只读活动列表请求；渲染版本阻止过期状态覆盖，重复读取次数未单独断言。
- 没有新增抽象记忆：过期控件的版本校验已落在控制器接口和自动化回归测试中。
- 回滚：撤回本分支 operations-flow Demo 前端改动即可恢复原导航；未改服务端、API 契约或持久化结构。
