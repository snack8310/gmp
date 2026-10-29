# 变更证据：活动与投放项编辑接管理 REST API

关联 issue #68；分支 `feat/68-campaign-activity-edit`，基于包含 #69 的 `origin/main` 提交 `3029965`。本机 Codex CLI：是。本轮只留下可提交 diff，未 commit、push、开 PR 或 merge。

## 背景与确认

活动的服务端真源和管理契约来自 #69。本轮用户已确认 REST 接入、失效引用处理、受众禁删依据、展示位规则、不改 `gmp-core` 和不做旧活动键的本地真源；见[任务卡](../../ai-loop/task-cards/68-campaign-activity-edit.md)。管理 API 无 CORS 响应头；示例启动时前端演示入口在 8081 提供同源页面与代理，管理服务可在 8082 保持原契约运行；前端监听端口可更换。未改 `management/` 或 `gmp-core/`。

## 关键实现

- `campaign-api-client.mjs` 封装契约的活动、投放项、动作、人群目录与占用查询路由。HTTP 非 2xx、网络错误和无效 JSON 向上抛；结构化日志只含方法、状态与耗时。
- #68 后续浏览器缺陷：用户提供的本机 headless Chrome 取证显示，`#campaign/activities` 报「无法连接活动管理 API」，同页直接 `fetch('/api/v1/campaigns')` 返回 200。客户端原先把默认 `globalThis.fetch` 存为实例方法，再以实例为接收者调用；严格接收者回归用例在旧实现报「无法连接」，修复后通过。现在只将默认 fetch 绑定到 `globalThis`，显式注入的 request 与网络异常转译逻辑不变。
- #68 后续修复：浏览器默认使用当前 HTTP/HTTPS 页面的同源基址，保留 `window.CAMPAIGN_API_BASE` 显式覆盖；离线单文件没有同源时直接报告 API 不可用，不发出请求。Node 默认基址仍为 `http://127.0.0.1:8081`，显式构造参数仍可指定其他端口。
- 活动列表、详情、创建、改名、删除及投放项增删改走 API；详情与编辑在进入路由时重新 GET，服务端分配的 ID 用于深链。活动流程只读；删被边引用的投放项展示服务端冲突。取消编辑不写入。
- 编辑器使用 API 动作目录、受众 `store.catalog()` 和 API 引用目录；即时校验一动作一人群、可用性、展示位与同位互斥，服务端仍作最终校验。失效引用显示已知名称和原 ID，不能原样保存。
- 受众删除前重新读取服务端活动和 `audience-reference-usages`；请求失败时不执行本地删除。活动写入成功后刷新占用提示，失败或取消不调用刷新。静态 `BROWSE_ACTIVITIES` 只留给旧只读渲染用例，运行时不用于活动真源或禁删。
- 独立评估首轮指出服务端目录空时误标可用、旧成功提示留在失败编辑页、乱序占用响应覆盖新提示；已分别以“目录显式空数组”、连续成功后 4xx、并发反序完成的回归用例复现，再修复。旧占用请求现在等待最新请求结果，不提交过期提示。
- 独立评估第二轮指出投放项写入等待期间切换路由会清空当前活动，成功回调再读取活动 ID 时抛错，导致受众占用不刷新。回归用例先复现，再在发请求前固定 ID 与目标路由；写入成功后仍刷新占用，仅在原路由仍有效时显示状态和导航。
- 独立评估第三轮结论为 `NEEDS-EVIDENCE`：未发现可取证的实现阻断，但真实 HTTP 与浏览器证据缺失。其非阻断风险指出切到非活动域时旧写入可能把页面带回活动；新用例先复现，再让所有域导航都递增活动路由序号，写入完成后仍刷新占用且不劫持当前页面。
- #68 后续的浏览器默认基址小修复经独立只读评估为 `PASS`；该裁决只覆盖本次小修复，原 #68 的真实 HTTP 与浏览器验收缺口仍保留。
- 旧键 `gmp-campaign-demo-v1` 本轮不读取、不写入、不清空、不自动迁移；冲突时服务端状态优先。离线导出明确提示 API 不可用时活动不能读取或保存。

## 验证结果

以下为本机实际执行记录；命令在仓根，除注明 `management/` 外。前端测试、语法、导出、文档和 diff 检查于本次接收者修复后执行；管理服务和 HTTP 检查是此前 #68 在沙箱中的失败记录，本次没有重跑。

| 命令 / 检查 | 退出码 | 证据摘要 |
|---|---|---|
| `node --test frontend/demos/operations-flow/*.test.mjs` | 0 | 默认 fetch 严格接收者、显式注入、真实网络异常提示、活动编辑与受众回归通过；新增用例在修改客户端前退出码 1，报「无法连接活动管理 API」。 |
| 对本轮改动的 `.mjs` 逐个 `node --check` | 0 | `campaign-api-client.mjs` 与 `campaign-api-client.test.mjs` 语法检查无诊断。 |
| `node frontend/demos/operations-flow/export.mjs /tmp/gmp-audience-demo.html` | 0 | 生成单文件。 |
| `python3 test/docs/consistency_check.py` | 0 | 文档一致性门禁通过。 |
| `git diff --check` | 0 | 已跟踪 diff 无空白错误；新文件仍须纳入提交前复查。 |
| `cd management && GOCACHE=${GOCACHE:-/tmp/gmp-68-go-build-cache} go run ./cmd/campaign-api -addr 127.0.0.1:8082` | 1 | 沙箱拒绝监听：`bind: operation not permitted`。 |
| `node frontend/demos/operations-flow/serve.mjs 8081 8082` | 1 | 沙箱拒绝监听：`listen EPERM`。 |
| `node frontend/demos/operations-flow/campaign-http-check.mjs http://127.0.0.1:8082` | 1 | 服务无法启动，客户端明确抛出无法连接；真实 HTTP CRUD 未在沙箱内验证。 |

执行器在沙箱外补证（2026-10-29，北京时间）：

| 命令 / 检查 | 退出码 | 证据摘要 |
|---|---:|---|
| 执行器沙箱外：`/tmp` 构建的 `campaign-api -addr 127.0.0.1:18182` + `node frontend/demos/operations-flow/campaign-http-check.mjs http://127.0.0.1:18182` | 0 | 真实进程 HTTP：目录、边引用删除 409 `edge_conflict`、创建活动/投放项 201、改引后占用查询随之变化、`missing_slot`/`unexpected_slot` 422、同包不同臂同位允许、普通组同位 409 `slot_conflict`、改名、删除 204、列表恢复原数量；未知详情 404。 |
| 执行器沙箱外：`serve.mjs 18181 18182` 同源代理 + 同一 `campaign-http-check.mjs http://127.0.0.1:18181` | 0 | 经同源代理同样通过（本机 8081/8082 被无关服务占用，故换端口）。 |
| 执行器沙箱外：headless Chrome 156（临时 profile，调试端口 19322）+ `browser-check.mjs 18181 19322`（修复前） | 1 | 页面显示「无法连接活动管理 API」而同页 `fetch('/api/v1/campaigns')` 返回 200；定位为默认 `fetch` 接收者错误（Illegal invocation），已由本机 Codex 修复并补回归。修复前错误提示未伪装成功。 |
| 执行器沙箱外：同上，修复后新 API 进程 + 新 profile | 0 | `create_activity`、受众删除被服务端占用阻止（「不能删除」）、`add_treatment`、`edit_and_delete`、`reload_without_deleted_activity` 全部 ok。 |

## 风险、缺口与回滚

- 真风险（已补证）：Codex 沙箱不能 bind；执行器已在沙箱外以真实进程完成 HTTP CRUD、占用变化与浏览器主路径（见上表），仍须独立评估器裁决。
- 接收者修复：沙箱外原生 headless Chrome 已复验通过（见上表）。浏览器矩阵、窄屏与真实浏览器中的深链前进后退仅部分覆盖（刷新复验已做）。
- 真风险（已用前端回归覆盖）：若浏览器仍请求固定 8081，换端口后会误连无关服务；若 `file://` 无同源时发出请求，离线页会给出误导反馈。本次分别覆盖当前页面同源地址与无请求的离线错误路径。
- 真风险（后续跟进）：#69 仅进程内保存，重启恢复构造种子；浏览器本地新增人群不在管理服务构造目录，不能直接保存到活动。服务端目录目前固定；若未来删去目录项且 API 活动资源不带历史名称，刷新后只能保留原 ID，历史名称需由服务端契约或版本化只读缓存提供。
- 伪风险：旧活动键损坏或版本未知不会破坏服务端活动，因为本轮完全不触达该键。
- 未讨论项交评估器：本地目录与服务端目录分叉及连续失败已用回归覆盖；真实浏览器切换深链与删除保护仍待可监听端口的环境验证。
- 外部读 / 写影响：浏览器通过本机 HTTP 读写管理 API 的当前进程内活动；受众与实验仍读写既有浏览器键。无生产数据库、缓存、消息队列、渠道或 `gmp-core` 变更。代理与客户端结构化日志不记录请求体、凭据或个人信息。
- 回滚：撤回本轮前端文件和 README、任务卡及本证据即可回到只读前端；服务端活动与旧活动键不受影响。最终合并仍由人工放行。
