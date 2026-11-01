# 变更证据：活动优先的演示工作区导航

关联 issue #74；分支 `feat/74-activity-first-navigation`。用户确认受众定义、实验定义和实验人群包继续作为跨活动共享资产。

## 背景

operations-flow Demo 原先把受众、实验、运营和组合放在顶层，活动列表藏在运营域内。现调整为最外层活动列表；选中活动后，在该活动上下文中进入四个领域页签。活动和投放项仍使用管理 API 的稳定 ID，未改变服务端契约和共享资产存储。

## 关键实现

- 新增统一活动路由解析与活动导航模块，支持活动 ID 编码、深链、旧运营活动链接映射、未知活动提示及浏览器历史。
- 新增独立 `activity-navigation.css`；核心 `style.css` 未修改。桌面侧栏包含活动列表和活动内页签，窄屏改为可横向访问的导航条。
- 活动详情的「编辑活动」和「＋ 添加投放项」锚点复用核心 `style.css` 中既有的 `.primary-link` 样式；只补 class，不改核心样式表，锚点仍按链接导航。
- 受众、实验和实验人群包仍读写原共享存储；组合继续引用当前模拟资产，不新增活动关联。
- 运营控制器根据路由活动 ID 读写活动与投放项。每次渲染生成版本号；切换路由时先使旧版本失效，旧表单提交、活动删除和投放项删除入口会拒绝过期事件。
- README、离线导出、服务端静态文件白名单和浏览器检查脚本同步支持新的导航结构。

## 关键文件

- `frontend/demos/operations-flow/index.html`：活动外层导航和活动内工作区结构。
- `frontend/demos/operations-flow/activity-routing.mjs`、`activity-navigation.mjs`：路由和 API 活动列表状态。
- `frontend/demos/operations-flow/activity-navigation.css`：独立导航样式；核心 `style.css` 保持不变。
- `frontend/demos/operations-flow/app.mjs`、`campaign-controller.mjs`、`campaign-ui.mjs`：活动上下文渲染和过期写控件保护。
- `frontend/demos/operations-flow/app.test.mjs`、`activity-routing.test.mjs`、`activity-navigation.test.mjs`、`campaign-activities.test.mjs`：活动切换、共享资产、深链、过期控件及活动操作锚点样式回归。
- `docs/ai-loop/task-cards/74-activity-first-navigation.md`：范围、验收、高风险和时间顺序记录。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `node --test frontend/demos/operations-flow/*.test.mjs` | 通过 | 前端回归无失败；覆盖路由、共享资产、CRUD、异步切换、过期提交/删除控件、活动操作锚点样式与离线导出。 |
| 对 `frontend/demos/operations-flow/*.mjs` 执行 `node --check` | 通过 | Demo 模块语法检查无诊断。 |
| `python3 test/docs/consistency_check.py` | 通过 | 文档一致性门禁通过，新增本证据后复跑。 |
| `node frontend/demos/operations-flow/export.mjs /tmp/gmp-activity-navigation-final.html` 与导出内容检查 | 通过 | 生成单文件；包含内联活动导航、核心 `.primary-link` 样式和控件版本校验逻辑。 |
| `git diff HEAD^ HEAD --check` 与范围检查 | 通过 | 无空白问题；未修改核心 `style.css`、`management/` 或 `gmp-core/`。 |
| 主 Agent CUA 桌面、窄屏、键盘、历史与活动详情预览 | 通过 | 活动上下文与四个页签可用；窄屏导航、焦点状态和浏览器前进/后退符合预期；活动详情中两个主要操作呈按钮式链接。 |
| 独立只读评估器 | `PASS` | 固定 SHA `c9724a27d35979625fed1446778e0335c11c8dd5`；无阻断问题。 |

## 风险与未覆盖项

- 高风险标准 3 命中：活动路由与 CRUD 事件的并发/执行顺序控制。用户于 2026-11-01 确认按高风险继续。确认晚于首个实现提交；该流程偏差未伪记为事前确认。独立评估将其列为非阻断治理风险，并只放行进入最终人工审核。
- 真实 Go 管理 API CRUD / HTTP 集成未运行：本机 Go 工具链版本低于项目要求，下载要求版本失败。浏览器预览使用本地只读模拟 API；不将其当作真实 API 验证。
- 独立评估器未重放真实浏览器走查；评估器运行环境无法完成写临时目录的导出测试。主 Agent 提供的全量前端测试、导出和浏览器走查结果作为证据审查。
- Safari、Firefox 与极窄屏设备未覆盖。程序化导航设置哈希后也主动渲染，可能重复发起只读活动列表请求；渲染版本阻止过期状态覆盖，独立评估认为非阻断。
- 没有新增抽象记忆：过期控件的版本校验已落在控制器接口和自动化回归测试中。
- 回滚：撤回本分支 operations-flow Demo 前端改动即可恢复原导航；未改服务端、API 契约或持久化结构。
