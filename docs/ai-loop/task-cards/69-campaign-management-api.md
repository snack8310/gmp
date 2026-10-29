# #69 活动管理 REST API 任务卡

## 任务分类与本质

- 类型：新需求开发；等级：大任务。新增被客户端依赖的 HTTP 管理契约；命中 `risk/high`（跨活动与人群引用写入边界）。
- 表面请求：对标 Momentum 的活动管理 REST。真实问题：#68 前端 `gmp-campaign-demo-v1` 只能过渡，活动保存、投放项校验与人群占用查询没有共同的服务端写入边界。
- 硬约束：服务端是 V1 活动/投放项状态真源；进程内存储+构造种子；默认不改 `gmp-core` 公开契约；不接生产 DB/Redis/真实渠道；中文主名「活动」「投放项」「人群组」「实验人群包」「活动流程」「展示位」；合入前产品再问用户放行。
- 已否决：以浏览器 localStorage 为最终真源；照搬 Momentum `POST .../update|delete` 动词路径。
- 成功标准：issue #69 验收标准 1–8 由本机 HTTP 正反例与 `go build/test/vet`、格式、文档门禁证明。

## 验收标准（承接 issue）

1. 本地启动后，独立 HTTP 客户端可完成资源导向 JSON API（最终路径写入 `docs/api/campaign-management-api.md`）：
   - `GET/POST /api/v1/campaigns`
   - `GET/PUT/DELETE /api/v1/campaigns/{campaignId}`（详情含投放项）
   - `POST /api/v1/campaigns/{campaignId}/treatments`
   - `PUT/DELETE /api/v1/campaigns/{campaignId}/treatments/{treatmentId}`
   - `GET /api/v1/actions`、`GET /api/v1/audience-references`
   - `GET /api/v1/audience-reference-usages?type=…&groupId=…&bundleId=…`
2. 连续请求由同一进程持有状态；改名不改稳定 ID；删除后一致；删空后种子不在该进程重生；前端 localStorage 不能覆盖服务端；重启重置有文档+测试。
3. 写入校验稳定唯一 ID、归属，以及「一个投放项恰好一个已知动作 + 一个可用人群」；普通组按 groupId、包臂按 bundleId+groupId；缺失/重复/多动作/未知或不可用引用 → 可判别 4xx，原状态不变。
4. 展示类动作必须带有效展示位；推送不得带展示位。同活动同展示位无决策表时，仅人群可证互斥才并存；同包不同臂有允许用例，无法证明互斥有拒绝用例；推送不参与互斥。
5. 投放项增删改均校验修改后整份活动，失败不留半份；删除被活动流程边引用的投放项须拒绝（本期不提供边编辑器，也不做同次原子去边，除非实现更简单且测试覆盖）。
6. `audience-references` 来自服务端构造/演示目录种子，含普通组与已发布包臂及可用性；**不**读浏览器 localStorage。
7. 引用占用查询随保存/改引/删除立即变化；**本期不宣称**能强制阻止浏览器侧受众删除。
8. HTTP 正反例覆盖 CRUD、重启重置、普通组/包臂、失效引用、展示位、同位互斥、悬空边、失败不污染、占用变化；`go build ./...`、`go test ./...`、`go vet ./...`、格式与文档门禁；记录结果与未覆盖项。

## 范围、影响与非目标

- 范围内：新建管理侧 Go 包/薄 HTTP 适配（建议仓根 `management/` 独立 module，或等价新包）；进程内 store；构造动作/人群目录；接口文档；任务卡；变更证据；HTTP 测试与启动入口。
- 非目标：#68 完整前端编辑体验；活动流程边编辑器；生产 DB/Redis/渠道；多用户同步；复制 Momentum URL 动词或发布缓存链路。
- 外部读写：HTTP 客户端读写本地服务；服务端只读写进程内状态。结构化日志不含凭据与个人敏感信息。
- 默认不改 `gmp-core` 公开契约。若发现必须改，**停止并报告**，不得擅自改。

## 未讨论项 → 推荐默认（用户已确认按推荐推进）

| # | 问题 | 推荐默认 | 状态 |
|---|---|---|---|
| 1 | 动作/人群目录如何初始化；#68 已编辑受众如何进目录 | 服务端用构造种子/演示目录初始化；**不**读 localStorage。#68 已编辑受众进服务端目录 → **本期不解决**，文档写明边界 | 已确认 |
| 2 | ID 谁分配 | **服务端分配**稳定唯一 ID；改名不改 ID | 已确认 |
| 3 | 受众禁删 | 本期只提供 `audience-reference-usages`；**不**宣称强制阻止浏览器侧删除 | 已确认 |
| 4 | 是否改 gmp-core | 不改公开契约；必须改则停下报告 | 已确认 |

## 已确认实施方案（建议，Codex 可微调但不得违反硬约束）

### A. 包布局

- 新建 `management/`（独立 `go.mod`：`github.com/snack8310/gmp/management`），**不**修改 `gmp-core` 公开类型/函数签名。
- 建议切分：`cmd/campaign-api` 启动；`internal/store` 进程内；`internal/catalog` 种子目录；`internal/domain` 活动/投放项校验；`internal/httpapi` 路由与处理器；`docs/api/campaign-management-api.md` 最终契约。
- 互斥证明 V1：同一实验人群包下不同臂（同 `bundleId`、不同 `groupId`）视为可证互斥；其余普通组之间无额外证明则不互斥。可不依赖 `gmp-core` 的 `ProvablyExclusive`，以免拖入完整 Definition 装配；若轻量只读复用且不改 core 公开契约亦可。

### B. 资源与校验

- 创建活动/投放项：请求体不要求客户端 ID；响应返回服务端分配 ID。
- 更新可改名称与投放项字段，不可改 ID；跨活动归属校验。
- 动作目录含 `requiresSlot`（或等价）以区分展示/推送；展示动作写入须带非空展示位；推送带展示位 → 4xx。
- 整份活动校验失败 → 整次写入回滚。
- 活动可携带只读/可原样保存的 `activityFlow`（若创建时带边）；删除被边引用的投放项 → 409/4xx 说明冲突。
- 种子：启动时装入演示动作、普通组、已发布包臂（ID 可对齐 frontend demo：`group-new`/`group-recall`、`bundle-warmup` 臂等）；**不**自动重生已删除的活动种子进当前进程（验收 2：删空后种子不在该进程重生——即活动列表种子若有初始样例活动，删除后不自动加回；目录种子可保留）。

### C. 错误与日志

- 非空与错误响应均为 JSON；错误可判别（缺字段 / 未知引用 / 不可用 / 互斥 / 悬空边 / 未找到）。
- 日志结构化，无凭据、无 PII。

## 高风险项

| 判定 | 命中 | 依据 |
|---|---|---|
| 通用 2、7 | 是 | 新增 HTTP 契约；跨活动与人群引用写入 |
| 通用 1、3、4、5、6 | 否/观察 | 不改 core 领域状态机；进程内非生产持久化；无授权体系本期 |
| 项目 4、15 | 观察 | 演示层人群引用与展示位互斥裁决在管理写入边界 |
| `risk/breaking` | 否 | 新增接口，不破坏既有公开契约 |

- 实施前人工确认：用户已确认选 C（先 API）及四条推荐默认；合入前产品再问用户放行。
- 破坏性变更：无（不改 gmp-core 公开契约）。

## 风险三档

- 真风险·阻断：服务端若信任前端校验而不自核，绕过浏览器即可写入错误活动 → HTTP 反例证明拒绝且状态不变。
- 真风险·仅观察：进程内重启丢失（批准的 V1 边界）；受众删除仍在浏览器，占用查询不能强制禁删。
- 伪风险：未接 MySQL/Redis/真实渠道。
- 未讨论项（已默认）：见上表；文档写明 #68 受众同步边界。

## 历史规则命中与扫描

扫描 `docs/history/memory/` 与近同域 changes（`2026-10-28-frontend-bundle-publish-catalog`、`slot-push-display-split`、`campaign-activities-browse`、`experiment-audience-bundle`）；规格：`interaction-invariants.md` 不变量 9、issue #69。

| 来源 | 命中 | 应用 |
|---|---|---|
| mechanization-claims | 部分 | 真实跑命令；断言用不同期望值 |
| review-package-consistency | 是 | 任务卡确认态与 PR 一致；合前停问 |
| source-enumeration-fidelity | 是 | 验收项落入测试 |
| shell-script-portability | 否 | 不改 shell 门禁脚本 |

## 最小验证集

- 在 `management/`（或实现所在 module）：`gofmt -l .`、`go build ./...`、`go vet ./...`、`go test ./...`（含 HTTP 正反例与重启重置）。
- 仓根：`python3 test/docs/consistency_check.py`、`git diff --check`。
- 必补：CRUD 状态持有、服务端分配 ID、展示位/推送、同位互斥允许与拒绝、失效引用 4xx、悬空边拒绝、占用查询变化、失败不污染。
- 未覆盖：生产持久化、多实例、#68 前端接线、受众服务端写入、浏览器矩阵。

## 评估器重点

绕过校验的写入；半份更新；重启是否被文档谎称持久化；localStorage 是否被读；gmp-core 公开契约是否被改；同包臂互斥与普通组互斥用例是否成对；合入前确认态是否与任务卡一致。
