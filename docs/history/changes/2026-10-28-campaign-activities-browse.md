# 变更证据：运营域活动浏览 Demo（只读）

关联：issue #60；规格 issue #59；里程碑「运营活动浏览 Demo（只读）」。基于 `7ad2027`（#58 tip）的 `feat/60-campaign-activities-browse`；实现后由开发执行器提交并开 PR。

## 背景

原运营页只解释活动、投放项和活动流程边的概念，无法沿活动实例查看关系。本次新增只读浏览，让读者从运营概念页进入活动列表与详情，同时维持组合页的装配和播放职责。

## 关键实现

- `#campaign` 保留原概念卡并新增列表入口；`#campaign/activities` 展示列表；详情按活动 ID 深链，未知 ID 显示未找到与返回链接。运营子路由高亮运营，离开组合仍暂停播放。
- 独立的本地构造目录包含动作、普通人群组和实验人群包臂；正常活动种子加载时校验唯一 ID、引用与同活动边端点。失效引用样例独立构造，页面显示原名称与 ID 及不可用标记。引用按 ID 判定，名称仅供展示。
- 详情分别展示投放项清单和活动流程关系表。RCS 成功到邮件、失败到短信的边只说明演示动作回执；不求值、不执行。空列表、空活动、无流程对象和无边分别提示。
- 列表链接对活动 ID 编码，子路由解码；无效编码进入未找到状态。离线导出纳入新增模块；没有新增持久化、外部接口或真实触达。

## 关键文件

- `frontend/demos/operations-flow/campaign-activities.mjs`、`campaign-ui.mjs`：构造数据、校验及只读列表和详情。
- `frontend/demos/operations-flow/app.mjs`、`index.html`、`style.css`：子路由、运营入口与表格样式。
- `frontend/demos/operations-flow/export.mjs`、`*.test.mjs`、`README.md`：离线导出、回归与操作说明。
- `docs/ai-loop/task-cards/60-campaign-activities-browse.md`：入口检查和验收基线。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `node --test frontend/demos/operations-flow/*.test.mjs` | 通过，退出码 0 | 覆盖种子校验、引用身份、异常引用、列表详情、编码深链、空态、组合暂停、原受众/实验行为及离线导出。 |
| 改动 JS 模块与测试逐个 `node --check` | 通过，退出码 0 | 新数据/UI 模块、路由、导出和对应测试均已检查。 |
| `python3 test/docs/consistency_check.py` | 通过，退出码 0 | 文档一致性门禁实际执行。 |
| `git diff --check` | 通过，退出码 0 | 已跟踪改动无空白错误。 |
| 本地 HTTP 请求 `/`、`/campaign-activities.mjs`、`/campaign-ui.mjs` | 返回 200 | 静态服务提供入口与新增模块；不代表浏览器交互验证。 |
| 独立只读评估器 | `PASS` | 核对任务卡、规格、实现和测试；独立探查特殊 ID、目录改名、失效原 ID。 |
| 浏览器桌面、390px、键盘与原生历史导航 | 未验证 | 浏览器控制服务返回 `The browser access host is unavailable`，Chrome 窗口连接返回 `cgWindowNotFound`。 |

## 风险与未覆盖项

- 真风险（仅观察）：实际浏览器表格横向滚动、桌面及 390px 布局、键盘焦点和原生前进后退缺少走查；处理器测试与 CSS 静态检查不能替代。进入人工最终审核时须明确保留此缺口，后续在可用浏览器中补验。
- 本次不覆盖：活动或投放项 CRUD、发布、权限、真实数据与服务接入、执行引擎、流程图编辑、跨活动边、指标和跨浏览器矩阵。
- 外部系统影响：新增活动浏览只读取本地静态构造数据；无 API、数据库、缓存服务、消息队列或渠道读写。既有受众/实验浏览器 localStorage 行为与格式未改；GitHub 写入限于本 issue 对应分支与 PR；无其他外部系统写入。
- 目录隔离：未改 `gmp-core/`、`docs/domain/`，未改变 #57/#58 已有验收语义。
- 回滚：撤回本次前端与文档 diff 即可；无迁移、回填或新缓存格式。
- 后续动作：人工审核并放行 PR；在浏览器工具可用时补原生走查。
