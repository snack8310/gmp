# 变更证据：前端实验人群包发布与统一引用目录

关联：issue #66；实施前人工确认见[任务卡](../../ai-loop/task-cards/66-frontend-bundle-publish-catalog.md)。分支已 commit 并推送；PR [#67](https://github.com/snack8310/gmp/pull/67) 已开；送审 SHA 以该 PR 当前 `head` 为准（勿用祖先提交绿灯充数）；**尚未 merge**，合入前仍须产品/用户放行。

## 背景与实现

- 原活动详情以静态普通组、包臂 catalog 判可用，无法反映受众和实验页修改。现在受众 store 保存普通人群定义、实验显式运行态及已发布包，并派生统一引用目录。`REFERENCE_CATALOG` 只保留动作构造项；活动详情从同一 store 目录按普通组 ID 或包 ID 加组 ID 解析。
- 受众侧独立「发布实验人群包」入口，绑定已保存且运行中的实验与同来源完整 A、B、对照臂。发布前拒绝缺臂、重复份号或 ID、异源及无效引用。已发布包依赖的人群不得改来源或实验引用，避免写出刷新后无法读取的缓存；显示名仍可修改。默认构造数据中普通组与包臂 ID 对齐既有活动样例，但样例仍经 store 目录判定；仅保存 `experimentId` 的旧人群仍只是普通组。对照臂保留在目录，不要求投放项。
- 实验页提供运行与停用。包记录发布时的实验代际；运行态切换推进代际，停用使旧包臂不可用，再运行也不复活旧包。目录缺失或不可用时，活动详情保留原引用 ID、名称并标不可用；被活动引用的包及普通组禁止删除，包所依赖的人群与实验也受保护。
- 本地缓存仍使用 `gmp-audience-demo-v1`，写入版本为 v4。读取 v1–v3 时，缺 `bundles` 取空数组，缺运行态取 `stopped`；旧键在下一次成功保存前不改写。损坏、未知版本或存储不可用时明确提示并退回会话内存，不删除原键。浏览器存储读写只记录结构化操作结果，不记录载荷。
- 受众统一目录、实验运行态、活动详情均由当前浏览器本地状态驱动；离线导出继续内联现有模块。未改 `gmp-core` 或生产系统。

## 关键文件

- `frontend/demos/operations-flow/audience.mjs`：缓存兼容、运行态、包发布校验、目录派生、删除保护。
- `frontend/demos/operations-flow/campaign-activities.mjs`、`campaign-ui.mjs`：活动样例与目录解析、失效引用展示。
- `frontend/demos/operations-flow/audience-ui.mjs`、`experiment-ui.mjs`、`app.mjs`、`index.html`：发布、目录、运行态和跨页只读消费。
- `frontend/demos/operations-flow/*.test.mjs`、`browser-check.mjs`：发布拒绝条件、目录身份、失效/禁删、历史缓存、深链、离线导出及真实 Chrome 主路径。
- `frontend/demos/operations-flow/README.md`、本文件、任务卡：操作口径、确认状态与验证证据。

## 验证结果

以下命令均针对本工作区执行；未将处理器替身测试写作浏览器走查。

| 命令 / 方式 | 实际结果 | 覆盖摘要 |
|---|---|---|
| `node --test frontend/demos/operations-flow/*.test.mjs` | 退出码 0 | 发布校验、运行态与代际、统一目录、普通组及包臂活动引用、已发布人群修改保护与重载、禁删、v1–v4 缓存兼容、受众/实验/活动深链、离线导出及原模拟回归。 |
| 对 demo 下 `.mjs` 逐个 `node --check` | 退出码 0 | 包含 audience、audience-ui、experiment-ui、campaign-activities、campaign-ui、app、export 及测试。 |
| `python3 test/docs/consistency_check.py` | 退出码 0 | 文档一致性门禁。 |
| `git diff --check` | 退出码 0 | 已跟踪 diff 无空白错误；新增文档另以 `git diff --no-index --check` 检查。 |
| `node frontend/demos/operations-flow/export.mjs /tmp/gmp-audience-demo.html` | 退出码 0 | 离线单文件生成；导出测试在 VM 中实际启动页面并核对活动深链。 |
| 本机 HTTP + `node frontend/demos/operations-flow/browser-check.mjs 18966 18969` | 退出码 0 | 实际浏览器完成目录初态、发布并落 v4、已发布人群修改保护、活动普通组与包臂引用、停用失效、再运行不复活、被引用包禁删、刷新一致、受众/实验深链。 |

浏览器复现：本机 `8080` 已被其他服务占用，按 README 的启动命令在该端口得到 `Address already in use`；改用 `python3 -m http.server 18966 --bind 127.0.0.1 --directory frontend/demos/operations-flow`，再用本机 Chrome headless 的远程调试端口 `18969` 运行仓内 `browser-check.mjs`。脚本通过 Chrome DevTools Protocol 调用真实页面的表单、按钮、hash 导航和刷新；每一步对页面文本及 localStorage 作断言并输出结构化结果。浏览器控制服务返回 `The browser access host is unavailable`，因此改用上述本机浏览器自动化。

## 风险、影响与未覆盖

- 真风险（阻断，已处理）：活动详情误用静态 catalog 时停用后会继续显示有效；现由 store 目录判定并以旧活动引用验证。旧缓存被升版清空时演示数据会丢失；现用 v1–v4 读取与损坏原键不覆盖测试验证。独立评估器首轮发现修改已发布包依赖的人群实验引用后能写出不可重载缓存；保存边界现与加载校验一致，拒绝此类修改，并有保存后重载回归。
- 真风险（仅观察）：浏览器本地数据不跨设备、账号或多用户同步；运行态和包发布不是生产发布。本轮未做桌面/窄屏视觉排版、键盘与跨浏览器矩阵走查；Chrome headless 主路径不能证明这些布局与可访问性细节。
- 伪风险：无生产数据迁移、真实触达或核心领域契约修改。
- 未讨论项：包级手动停用不在本期范围；以实验停用传播失效。分流修订编辑器与比例/白名单编辑器未提供；运行参数调整与修订的区别仅保留既有领域口径，不由 demo 发明修订行为。
- 浏览器中新发布的包会进入统一目录，但现有只读活动样例仍引用默认包；新包并不自动挂到活动。构造活动测试证明包臂引用身份可跨活动复用；活动装配仍是本期非目标。
- 外部读写影响：读取 GitHub issue #66；浏览器只读写本地 `localStorage` 的演示键，并在本机 HTTP 服务读取静态文件。没有数据库、缓存服务、消息队列、渠道、生产配置或其他外部写入。回滚前端与文档 diff 即可；已在浏览器写入的 v4 演示缓存可由旧版页面视为未知版本，本轮未执行生产变更或数据删除。

## 评估器与放行

独立只读评估器首轮裁决 `FAIL`，发现保存已发布包依赖人群时可写出不可重载缓存；已补保存边界与回归。复审裁决 `PASS`，评估器复现该修改现被拒绝且不写缓存，改显示名后重载成功；建议无需新增文档记忆，反例已由保存边界与自动化回归固化。Chrome 与导出结果由主 Agent 执行，评估器审查其脚本及证据；桌面/窄屏视觉排版、键盘及跨浏览器仍未覆盖。大任务最终人工审核及合入前产品侧放行均**待确认**。送审以 PR head 为准；合入前再问用户。
