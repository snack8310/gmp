# 领域演示：受众、实验、运营与组合

独立前端展示 Demo：受众、实验和组合仍在浏览器中演示；活动与投放项经本机 #69 管理 REST API 读写。全部为构造数据，不接生产服务，不依赖 `gmp-core`、WebGL、框架、CDN 或第三方资产。

## 运行与导出

活动编辑须同时启动管理 API 和同源演示入口。在两个终端分别运行：

```sh
cd management && GOCACHE=${GOCACHE:-/tmp/gmp-68-go-build-cache} go run ./cmd/campaign-api -addr 127.0.0.1:8082
node frontend/demos/operations-flow/serve.mjs 8081 8082
```

打开 `http://127.0.0.1:8081`。浏览器默认把当前 HTTP/HTTPS 页面的 `window.location.origin` 用作 API 基址；载入 `app.mjs` 前设置 `window.CAMPAIGN_API_BASE` 可覆盖。Node 客户端可向 `CampaignApi` 构造器传入基址，未传时仍默认 `http://127.0.0.1:8081`。#69 服务不提供跨源响应头，演示入口在同一源代理 `/api/v1/*` 到 8082；HTTP 写入仍由 #69 服务执行。只用普通静态服务启动页面时，没有 `/api/v1/*` 代理，活动 API 不可用。端口占用时可给 `serve.mjs` 传其他监听端口和 API 端口，浏览器无需再配置默认基址。

直接打开的单文件版：

```sh
node frontend/demos/operations-flow/export.mjs /tmp/gmp-audience-demo.html
```

生成器内联同版本样式与模块，不上传、不部署；HTML 可离线打开。受众、实验和组合可离线演示；`file://` 没有 HTTP/HTTPS 同源基址，活动页默认明确提示 API 不可用，不会读取或保存活动。Node 用于演示服务、测试与导出。

## 操作

### 受众

- 受众页管理可复用人群：名称搜索、新建、编辑、删除、取消、保存与实时预览。删除前重新读取服务端活动和引用占用；读取失败时拒绝删除。被服务端当前活动或已发布包引用的人群组不能删除；删除未被引用且当前组合使用的人群时，组合会切到剩余的首个人群，或清空引用。进入方式包含批量来源及手动/每日 UTC 排期，或事件来源及预置模拟触发事件；国家、生命周期、未活跃天数按 AND 圈选。
- 可选引用**实验页已保存**的实验，本页只选 ID，不创建、改名或调整比例。匹配预览展示固定模拟池的资格人数和匿名样本；零匹配可保存，组合页不能播放触达。
- 「发布实验人群包」是独立动作：选一份已保存、引用运行中实验的人群，按同一来源发布完整 A、B、对照臂。包与臂各有稳定 ID；对照臂可不挂投放项。仅保存带实验 ID 的人群不会生成包臂。下方统一引用目录列出普通人群组和已发布包臂的身份、名称、类型与可用性；被活动引用的包不能删除。
- 点击「引用到组合」带入已保存人群。草稿不影响保存定义或组合；组合的「编辑此人群」返回本页。

### 实验

- 实验页集中创建、查看、改名、运行、停用与删除实验。被已保存人群或包引用的实验不能删除，须先解除引用。改名不改变 ID 或稳定分配。新建实验初始为「已停用」；运行后才可发布包。停用使旧包臂失效，再运行也不会复活旧包；需重新发布。
- 比例固定为 A 45% / B 45% / 对照 10%。同一实验 ID 与模拟用户始终落在同一份；不同实验 ID 使用不同演示盐。小样本实际比例可能偏离目标值。染色和放量只有只读说明，不提供编辑器或指标界面。
- 人群、实验运行态与已发布包仍保存在本浏览器 `gmp-audience-demo-v1` 命名空间。可读旧版 v1–v3 缓存；缺少包按空目录解释，缺少运行态一律按「已停用」解释。兼容载入时不改写原键，下次成功保存才写 v4。损坏、禁用、未知版本或写入失败时提示并退回会话内存，不静默删除原键；刷新可能丢失会话修改。

### 运营

- `#campaign` 保留概念卡；活动列表从 `GET /api/v1/campaigns` 读取，详情使用稳定 ID 深链。可创建、改名、删除活动，增删改投放项；取消编辑不写 API。活动与投放项 ID 均由服务端分配，删除整活动同时删除其活动流程边；被边引用的投放项删除会收到冲突提示。活动流程只读展示。
- 每个投放项选一个 API 动作和一个 `store.catalog()` 人群，保存前同时核对 API 引用目录；展示动作须选择展示位，推送不得带展示位。同一活动同展示位仅同包不同臂可证互斥。服务端仍会对每次写入重校验，非 2xx 或网络错误留在编辑页显示，不能冒充保存成功。
- 普通组按组 ID、包臂按包 ID 与组 ID 识别。缺失或不可用引用保留服务端 ID，以本地或 API 目录已知名称显示并标不可用，编辑时须改选可用引用。浏览器本地新增的人群不自动进入 #69 的构造目录，故不能直接用于活动保存；后续需单独解决目录同步。
- `campaign-activities.mjs` 的静态构造活动只保留给旧只读渲染测试作对照，运行时活动列表和详情均不读取它。#69 进程重启会恢复构造种子，编辑结果不持久化到磁盘。
- 旧活动键 `gmp-campaign-demo-v1` **不读取、不迁移、不写入、不清空**；服务端与旧键冲突时以服务端为准。损坏或未知版本原样留在浏览器，不能覆盖服务端活动。回退到旧只读前端不改变该键；若未来要迁移，应另行设计版本校验、冲突解决和失败回退。受众键 `gmp-audience-demo-v1` 仍按原有兼容规则使用。

### 组合

- 组合页引用已保存人群并提供场景、SVG 沙盘、播放、暂停、重播、时间轴、倍速、节点详情和统计。切页或页面隐藏时暂停；切换人群、场景或流程实验节点会重置并暂停。
- 继承的人群实验只读。普通出口或继承 A/B 路径可添加一个进入后的流程实验节点，选择实验域已有项；主对照不触达、不能添加。节点只保存在当前会话，不写回人群。相同实验在继承和流程节点中复用时沿用原分配。
- 流程实验节点支持路径点选、检查器改引用及删除；沙盘不提供通用 Flow 图编辑。减少动态效果偏好下不绘制流动小人，主动播放仍更新统计。窄屏可通过按钮调整视角并横向选择节点。

## 口径与边界

- 模拟样本池固定为 240 个虚构记录，App 与白名单为互斥来源。国家、生命周期和未活跃天数都是本地生成的演示属性，不声称属于生产字段或口径。
- 匹配预览是此固定数据集的精确计算，不是生产预估；同一规则用于预览、列表计数与沙盘引用。
- 总人群 = 未进入 + 处理中 + 触达成功 + 触达失败 + 对照留存。处理中包含路上和等待回执，回执延迟不是失败。
- 节点累计到达不能跨节点相加为总人数；当前占用包含节点和其出站路段。主、副对照留存合计为不触达终态。
- 成功仅指模拟回执，不是阅读、下单或转化。渠道和结果为展示脚本，优化场景不是效果预测。
- 排期与触发类型仅演示配置；播放仍只模拟符合条件样本依次进入，不执行调度、事件订阅、重复事件处理或多画布分发。多画布事件归属与投递语义尚未确定，本 Demo 不替产品作决定。
- 不上传名单、不保存样本、不记录生产执行或染色、不接真实分桶、不发任何消息。活动管理仅有本机进程内后端，没有账号、权限、多用户同步或生产执行引擎。
- 统一目录和运行态只在当前浏览器本地演示；不跨设备同步，不代表生产发布或生产可用性判定。
- 这是展示沙盘，不是生产平台的流程引擎或领域语义实现。领域文档中尚待确认的功能不因本 Demo 变成业务决策。

## 验证

```sh
node --test frontend/demos/operations-flow/*.test.mjs
node --check frontend/demos/operations-flow/audience.mjs
node --check frontend/demos/operations-flow/audience-ui.mjs
node --check frontend/demos/operations-flow/experiment-ui.mjs
node --check frontend/demos/operations-flow/model.mjs
node --check frontend/demos/operations-flow/campaign-activities.mjs
node --check frontend/demos/operations-flow/campaign-ui.mjs
node --check frontend/demos/operations-flow/app.mjs
node --check frontend/demos/operations-flow/export.mjs
node --check frontend/demos/operations-flow/browser-check.mjs
node --check frontend/demos/operations-flow/campaign-api-client.mjs
node --check frontend/demos/operations-flow/campaign-controller.mjs
node --check frontend/demos/operations-flow/serve.mjs
node frontend/demos/operations-flow/campaign-http-check.mjs http://127.0.0.1:8081
```

macOS 上可用真实 Chrome headless 复现活动与受众主路径。先按上文启动 API 和同源演示入口，再在另一个终端用全新临时浏览器配置启动 Chrome，最后运行脚本（需要支持内置 `WebSocket` 的 Node）：

```sh
'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' --headless=new --remote-debugging-port=9222 --user-data-dir="$(mktemp -d)" --no-first-run --no-default-browser-check http://127.0.0.1:8081/
node frontend/demos/operations-flow/browser-check.mjs 8081 9222
```

Chrome 命令需保持运行；脚本的位置参数依次为 HTTP 端口与 Chrome 调试端口。脚本会在该临时浏览器配置中写入演示键，请勿指向日常浏览器配置。

测试覆盖四域导航、活动深链、编辑校验、API 失败、受众禁删、v1–v4 受众缓存兼容、导出启动，以及原有筛选、分组、流程节点、人数守恒、播放和低动态分支。真实 HTTP 脚本覆盖活动与投放项写入、错误体、占用变化；浏览器脚本覆盖界面主路径。JavaScript 检查目前手动执行，未宣称已纳入 CI。

真实渠道、生产性能、跨浏览器全矩阵不在范围。回滚本次前端文件即可；遗留缓存可忽略或仅移除该命名空间，不涉及业务数据。
