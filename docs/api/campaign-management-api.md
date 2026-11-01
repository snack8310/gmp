# V1 活动管理 REST API

关联 issue #69、#76、#78。该 API 用于本机活动演示，管理活动、唯一活动受众、投放项、活动流程入口与回执边，以及对应的可用目录。正式中文名使用「活动」「人群组」「实验人群包」「投放项」「活动流程」「展示位」。

## 启动与状态

在仓库根目录运行：

```sh
cd management
go run ./cmd/campaign-api -addr 127.0.0.1:8081
```

`-addr` 必填。服务端是本 V1 的活动及投放项状态真源。一个进程中的连续 HTTP 请求读写同一份受互斥锁保护的内存状态；启动时加载构造活动与目录种子，删除后当前进程不会重新加载。**进程重启会重置活动、投放项编辑及动态目录项，恢复初始种子**，没有磁盘持久化或多实例同步。客户端本地缓存不能覆盖服务端状态。

服务仅用于本机演示，不具备认证、授权或生产持久化能力；不要绑定公网地址或用于真实运营数据。服务不读取浏览器 `localStorage`，不访问数据库、Redis、消息队列或真实渠道。浏览器在加载时把本地共享人群的 ID 与名称写入 API 目录；圈选规则及模拟样本仍留在浏览器。HTTP 请求是唯一外部读写面：读请求只读进程内状态，写请求只写进程内状态。请求日志为结构化 JSON，仅记录方法、状态码和耗时，不记录请求体、凭据或个人信息。

## 资源模型

活动详情及列表元素：

```json
{
  "id": "campaign-branch",
  "name": "跨境召回",
  "audience": { "type": "group", "groupId": "group-recall" },
  "treatments": [
    { "id": "treatment-rcs", "name": "RCS 投放项", "actionId": "action-rcs" },
    { "id": "treatment-email", "name": "邮件投放项", "actionId": "action-email" },
    { "id": "treatment-sms", "name": "短信投放项", "actionId": "action-sms" }
  ],
  "activityFlow": {
    "id": "flow-recall",
    "entries": [
      { "groupId": "group-recall", "kind": "start", "treatmentIds": ["treatment-rcs"] }
    ],
    "edges": [
      { "id": "edge-success", "fromTreatmentId": "treatment-rcs", "toTreatmentId": "treatment-email", "conditionLabel": "成功" },
      { "id": "edge-failed", "fromTreatmentId": "treatment-rcs", "toTreatmentId": "treatment-sms", "conditionLabel": "失败" }
    ]
  }
}
```

`audience` 只允许一个普通人群组 `{ "type": "group", "groupId": "…" }` 或一个实验人群包 `{ "type": "bundle", "bundleId": "…" }`。普通组对应一个流程入口，可列一个或多个起始投放项；实验包须为每个臂配置一个入口，入口类型为 `start` 或显式的 `control`。对照入口的 `treatmentIds` 必须为空。`treatments[]` 只描述动作及可选 `slotId`，不携带人群。

活动流程对象包含 `id`、`entries[]`、`edges[]`。入口路由把唯一活动受众或包内各臂分配给起始投放项；回执边把动作成功 / 失败等契约结果连接到后续投放项。回执边目标沿用上游回执 UID，不重新枚举人群。单步草稿可以暂时没有流程或入口；完整活动 core 只接受合法的受众、入口流程与投放项。未完成管理草稿不能开始本地演练。

创建活动只接受 `{ "name": "活动名称" }`，返回含服务端分配 `id` 和空 `treatments` 的草稿。选择活动受众后才可新增投放项。创建投放项接受 `name`、`actionId` 和可选 `slotId`，不接受客户端 `id` 或投放项人群。活动及投放项 ID 均由服务端分配，在同一进程内唯一；改名不改 ID，删除后当前进程不复用 ID。重启后内存状态和分配序列重置，本 API 不保证跨重启唯一。

`PUT /campaigns/{campaignId}` 接受活动名称等基本字段；`id` 必须与路径一致，`treatments` 必须包含原有全部投放项 ID。活动受众只能通过 `/audience` 更新，流程只能通过 `/flow` 更新。若活动更新请求省略 `audience`，服务端保留当前受众。所有写入均在服务端重校验修改后的活动，通过后才替换状态。

## 路由与响应

| 方法与路径 | 请求 | 成功响应 |
|---|---|---|
| `GET /api/v1/campaigns` | 无 | `200 {"campaigns":[活动对象…]}`，按 ID 排序 |
| `POST /api/v1/campaigns` | `{ "name": "…" }` | `201` 活动草稿 |
| `GET /api/v1/campaigns/{campaignId}` | 无 | `200` 活动对象 |
| `PUT /api/v1/campaigns/{campaignId}` | 活动基本字段及完整 `treatments` 集合 | `200` 更新后的活动对象 |
| `PUT /api/v1/campaigns/{campaignId}/audience` | `{ "expectedAudience": 旧值或 null, "audience": 新值或 null }` | `200` 更新后的完整活动对象 |
| `PUT /api/v1/campaigns/{campaignId}/flow` | `{ "id", "expectedEntries", "expectedEdges", "entries", "edges" }` | `200` 更新后的完整活动对象 |
| `DELETE /api/v1/campaigns/{campaignId}` | 无 | `204`，空响应体 |
| `POST /api/v1/campaigns/{campaignId}/treatments` | `{ "name", "actionId", "slotId"? }`，不含 ID | `201` 投放项对象 |
| `PUT /api/v1/campaigns/{campaignId}/treatments/{treatmentId}` | 完整投放项对象 | `200` 更新后投放项对象 |
| `DELETE /api/v1/campaigns/{campaignId}/treatments/{treatmentId}` | 无 | `204`，空响应体 |
| `GET /api/v1/actions` | 无 | `200 {"actions":[…]}` |
| `GET /api/v1/audience-references` | 无 | `200 {"audienceReferences":[…]}` |
| `PUT /api/v1/audience-references/{groupId}` | `{ "name": "人群名称" }` | `200` 更新后普通组引用对象 |
| `DELETE /api/v1/audience-references/{groupId}` | 无 | `204`；被活动受众或流程入口引用时返回 `409` |
| `GET /api/v1/audience-reference-usages?type=group&groupId=…` | 普通组身份 | `200 {"usages":[…]}` |
| `GET /api/v1/audience-reference-usages?type=arm&bundleId=…&groupId=…` | 包臂身份 | `200 {"usages":[…]}` |

`PUT /audience` 使用服务端当前值与 `expectedAudience` 做比较；不匹配时返回 `409 audience_conflict`。当选择发生变化，服务端在同一锁内清空旧 `activityFlow.entries`，保留 `activityFlow.edges`，避免保存与新受众不匹配的旧入口。随后由 `/flow` 一次性保存新的入口与回执边。`PUT /flow` 必须同时回传刚读取的 `expectedEntries` 与 `expectedEdges`；任一过期即返回 `409 flow_conflict`，服务端不会部分写入。

动作目录元素为 `{ "id", "name", "requiresSlot", "available", "receiptOutcomes"? }`。`receiptOutcomes` 仅列动作契约明确声明的回执条件。人群目录元素为 `{ "type", "groupId", "bundleId"?, "name", "bundleName"?, "available" }`；实验包通过多条 `type=arm` 记录列出各臂。占用元素为 `{ "campaignId", "campaignName", "role" }`，`role` 为 `activity-audience`、`experiment-audience` 或 `flow-entry`，空占用为 `[]`。查询只扫描服务端当前保存的活动；普通组查询不得带 `bundleId`，包臂查询必须带 `bundleId`。

## 服务端校验

- 构造动作 ID：`action-rcs`、`action-email`、`action-sms` 为推送；`action-show-image` 是需展示位的展示动作；`action-retired` 为不可用反例。
- 构造普通组：`group-new`、`group-recall` 可用，`group-removed` 不可用。已发布包 `bundle-warmup` 的 `group-arm-a`、`group-arm-b`、`group-arm-control` 可用；`bundle-retired/group-arm-retired` 不可用。
- 有效展示位 ID：`slot-home-banner`、`slot-home-hero`。展示动作必须指定其中之一；推送及其他非展示动作不得指定展示位。
- 每个投放项恰好引用一个已知且可用动作。请求不接受未定义字段，因此旧投放项 `audienceRef` 不再是有效字段。
- 一个活动只能选一个已知且可用普通组或实验包。活动尚未选择受众时，不得添加投放项或入口路由。活动包入口必须逐臂覆盖；只有包内臂可标记 `control`，且对照臂不得绑定起始投放项。一个起始投放项不能同时属于多个实验臂。
- 同一活动内多个展示投放项占同一展示位时，须由同一实验包的不同臂证明互斥；其他冲突返回 `409 slot_conflict`。推送不参与展示位竞争。本 V1 不提供决策表编辑接口。
- 流程边端点必须在本活动，不能指向自己；边 ID、条件必须非空且不得重复。回执条件必须由来源动作声明；流程图不能有循环。删除被入口路由或现有边引用的投放项返回冲突，保留原状态。

初始活动种子为 `campaign-branch`、`campaign-single`、`campaign-empty`、`campaign-no-edges`，与前端演示常用 ID 对齐。`campaign-branch` 选择单一普通组并按 RCS 成功 / 失败回执分支；`campaign-no-edges` 选择一个实验人群包并将 A、B、对照臂分别路由。浏览器在加载时同步本地普通人群的 ID 与名称到 API 目录；新建或改名时同步当前引用。进程重启会丢失动态目录项，重新打开演示后会从浏览器本地定义重新写入。活动受众占用与投放项引用占用均由使用情况查询返回；删除接口会同时检查活动选择和流程入口，防止悬空引用。

## 错误语义

除 `204` 外，响应均为 JSON。错误统一为 `{ "error": { "code": "…", "message": "中文说明" } }`。失败写入不修改已保存的活动或投放项。

| HTTP 状态 | 典型 `code` | 含义 |
|---|---|---|
| `400` | `invalid_json`, `missing_field`, `invalid_audience_selection`, `expected_audience_required`, `expected_flow_required` | JSON、字段或并发写入前置状态无效 |
| `404` | `not_found` | 活动、所属投放项或路由不存在 |
| `405` | `method_not_allowed` | 路由存在但方法不支持 |
| `409` | `id_conflict`, `missing_activity_audience`, `audience_conflict`, `flow_conflict`, `flow_id_conflict`, `invalid_entry_route`, `entry_outside_audience`, `entry_conflict`, `edge_conflict`, `flow_cycle`, `slot_conflict`, `treatment_set_conflict` | 受众或流程并发冲突、引用关系或同位竞争无效 |
| `415` | `unsupported_media_type` | 写入请求不是 JSON |
| `422` | `unknown_action`, `unavailable_action`, `unknown_audience`, `unavailable_audience`, `missing_slot`, `unknown_slot`, `unexpected_slot`, `invalid_edge_condition` | 引用、展示位或动作回执契约不满足要求 |

## 验证与兼容边界

`management/internal/httpapi/http_test.go` 检验活动受众、流程入口 / 回执边、乐观冲突、动作与人群目录、展示位及进程重启语义；`frontend/demos/operations-flow/campaign-http-check.mjs` 检验前端通过 V1 创建并配置活动、实验臂路由和同位冲突。

本次对尚未合并的 `/api/v1` 原地破坏性更新，不增加 V2 或旧字段兼容层。旧 `audienceRefs[]`、`treatments[].audienceRef`、`/audiences` 及 `expectedAudienceRefs` 不再接受；仓库唯一演示客户端同步迁移。该变更仅影响本机演示 API，不涉及持久化迁移、生产数据、外部渠道或真实触达。
