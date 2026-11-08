# V1 活动管理 REST API

关联 issue #69、#76、#78。该 API 用于本机活动演示，管理活动、唯一活动受众、投放项、活动流程入口与回执边，以及对应的可用目录。正式中文名使用「活动」「人群组」「实验人群包」「投放项」「活动流程」「展示位」。

## 启动与状态

在仓库根目录运行：

```sh
cd management
export MYSQL_DSN
export MYSQL_MAX_OPEN_CONNS
export MYSQL_MAX_IDLE_CONNS
export MYSQL_CONN_MAX_LIFETIME_SEC
go run ./cmd/campaign-api -addr 127.0.0.1:8081
```

`-addr` 和上述 MySQL 设置都必填。`MYSQL_DSN` 必须指向平台自有 MySQL 中已创建的数据库；不要将凭据写入仓库或日志。启动时检查连接并应用仓库内的加法迁移；缺配置、连接失败、迁移失败或 schema 版本不受支持时进程退出，不回退到内存状态。活动、投放项、流程边、人群引用及 ID 分配序列写入平台 MySQL，进程重启后仍可读取。每次写操作在数据库事务与共享写锁内加载、校验并替换完整配置，保证多进程写入不会丢失并发更新。该实现目前面向演示与架构验证，整份配置读写会随数据量增长而变慢。

当前迁移只创建新的平台管理表，不修改或迁移旧数据；应用版本回退时保留 schema 与数据，不自动执行破坏性删除。删除这些表需要单独的变更计划与人工审批。

服务不具备认证或授权能力，不应直接公开或接入真实运营数据。当前它只写平台自有 MySQL 的管理配置，不连接人群用户池，不读写人群成员，不保存成员快照，不访问 Redis、消息队列或真实渠道。平台数据库与后续用户池来源应使用不同连接配置和授权；本 API 暂不创建用户池连接。浏览器仍在载入时同步共享人群的 ID 与名称，圈选规则及模拟样本留在浏览器。HTTP 与 MySQL 操作使用结构化日志，只记录方法 / 操作、结果与耗时，不记录 DSN、请求体、用户标识或业务载荷。

### MySQL 人群来源适配器（尚未接入 API）

`management.MySQLAudienceSource` 是外层只读查询适配器，不由当前 API 启动器创建。调用方必须单独建立来源数据库连接，并显式提供 `Table`、`UIDColumn`、`CursorColumn`、`Attributes` 映射、`PageSize`，以及确认 UID 唯一、游标唯一且稳定的 `UIDUnique`、`CursorUnique` / `CursorStable` 声明。表名只接受 `table` 或 `database.table`，列名只接受简单 SQL 标识符；初始化会验证关系、必填列和非空 UID / 游标列。唯一性和稳定性由来源契约提供，适配器不扫描整张表来推断它们。

`ReadPage` 使用游标分页，只选择 UID、游标和显式映射的属性列；它不提供写方法、不缓存来源成员、不记录成员、属性或游标值。游标唯一性 / 稳定性来自调用方对来源契约的确认，适配器不推断用户池表结构。它目前未接入 `/api/v1` 的人群求值流程；实际用户池 schema、可读字段、筛选规则、分页字段、连接配置来源和只读授权仍待确认，因此不能视为已接入真实人群。

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

创建活动只接受 `{ "name": "活动名称" }`，返回含服务端分配 `id` 和空 `treatments` 的草稿。选择活动受众后才可新增投放项。创建投放项接受 `name`、`actionId` 和可选 `slotId`，不接受客户端 `id` 或投放项人群。活动及投放项 ID 均由服务端分配，分配序列保存在 MySQL；改名不改 ID，删除后也不复用已分配 ID。浏览器本地缓存不能覆盖服务端状态。

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

新安装的 MySQL schema 不会自动写入活动、投放项、人群引用或浏览器草稿种子。活动与投放项需通过 API 创建；普通人群引用由浏览器在载入、保存或改名时同步到 API，并持久化在平台 MySQL。实验包臂引用仍由客户端目录提供，当前 API 不保存实验定义或分流记录。活动受众占用与投放项引用占用均由使用情况查询返回；删除接口会同时检查活动选择和流程入口，防止悬空引用。

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
| `500` | `storage_unavailable` | 平台 MySQL 读写、迁移或事务失败；API 不切换到内存状态 |

## 验证与兼容边界

`management/internal/httpapi/http_test.go` 检验活动受众、流程入口 / 回执边、乐观冲突、动作与人群目录、展示位及演示内存替身语义；`management/internal/management/mysql_store_integration_test.go` 使用 `GMP_TEST_MYSQL_DSN` 验证 MySQL 迁移、跨重启读取、跨实例 ID 分配和失败事务回滚。`frontend/demos/operations-flow/campaign-http-check.mjs` 检验前端通过 V1 创建并配置活动、实验臂路由和同位冲突。

本次对尚未合并的 `/api/v1` 原地破坏性更新，不增加 V2 或旧字段兼容层。旧 `audienceRefs[]`、`treatments[].audienceRef`、`/audiences` 及 `expectedAudienceRefs` 不再接受；仓库唯一演示客户端同步迁移。该变更仅影响本机演示 API，不涉及持久化迁移、生产数据、外部渠道或真实触达。
