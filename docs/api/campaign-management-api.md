# V1 活动管理 REST API

关联 issue #69。本接口只管理活动、投放项及其引用；中文产品名以「活动」「投放项」「人群组」「实验人群包」「活动流程」「展示位」为准。`Campaign`、`Treatment` 是已确认的代码术语，其余英文字段名只是本 API 的传输字段，不是产品正式英文定名。

## 启动与状态

在仓库根目录运行：

```sh
cd management
go run ./cmd/campaign-api -addr 127.0.0.1:8081
```

`-addr` 必填。服务端是本 V1 的活动及投放项状态真源。一个进程中的连续 HTTP 请求读写同一份受互斥锁保护的内存状态；启动时加载构造活动与目录种子，删除后当前进程不会重新加载。**进程重启会重置所有活动及投放项编辑，恢复初始种子**，没有磁盘持久化或多实例同步。客户端本地缓存不能覆盖服务端状态。

服务仅用于本机演示，不具备认证、授权或生产持久化能力；不要绑定公网地址或用于真实运营数据。服务不读取浏览器 `localStorage`，不访问数据库、Redis、消息队列或真实渠道。HTTP 请求是唯一外部读写面：读请求只读进程内状态，写请求只写进程内状态。请求日志为结构化 JSON，仅记录方法、状态码和耗时，不记录请求体、凭据或个人信息。

## JSON 资源

活动详情及列表元素：

```json
{
  "id": "campaign-branch",
  "name": "跨境召回",
  "treatments": [
    {
      "id": "treatment-rcs",
      "name": "RCS 投放项",
      "actionId": "action-rcs",
      "audienceRef": { "type": "group", "groupId": "group-recall" }
    }
  ],
  "activityFlow": {
    "id": "flow-recall",
    "edges": [
      {
        "id": "edge-success",
        "fromTreatmentId": "treatment-rcs",
        "toTreatmentId": "treatment-email",
        "conditionLabel": "成功"
      }
    ]
  }
}
```

示例只展示部分投放项和边；真实 `GET` 返回完整对象。无活动流程时省略 `activityFlow`。`treatments` 始终是数组。展示投放项另带 `slotId`；推送投放项不带。普通人群组引用只用 `type=group` 与 `groupId`；实验人群包臂只用 `type=arm`、`bundleId` 与 `groupId`，不能仅靠组名或 `groupId` 判断包臂身份。

创建活动只接受 `{ "name": "活动名称" }`，返回含服务端分配 `id` 和空投放项数组的活动。创建投放项接受 `name`、`actionId`、`audienceRef` 和可选 `slotId`，不接受客户端 `id`。活动及投放项 ID 均由服务端分配，在同一进程内唯一；改名不改 ID，删除后 ID 不复用。重启后内存状态和分配序列重置，本 API 不保证跨重启唯一。

`PUT /campaigns/{campaignId}` 接受完整活动对象，`id` 必须与路径一致，`treatments` 必须包含原有全部投放项 ID，各投放项字段可修改。新增、删除投放项走各自资源路由。若原活动带 `activityFlow`，`PUT` 必须原样回传；本期不提供边编辑，也不允许用 `PUT` 清除边。`PUT /treatments/{treatmentId}` 接受完整投放项对象，其 `id` 必须与路径一致。所有写入均在服务端重校验修改后的整份活动，通过后才替换状态。

## 路由与响应

| 方法与路径 | 请求 | 成功响应 |
|---|---|---|
| `GET /api/v1/campaigns` | 无 | `200 {"campaigns":[活动对象…]}`，按 ID 排序 |
| `POST /api/v1/campaigns` | `{ "name": "…" }` | `201` 活动对象 |
| `GET /api/v1/campaigns/{campaignId}` | 无 | `200` 活动对象，含投放项与可选活动流程 |
| `PUT /api/v1/campaigns/{campaignId}` | 完整活动对象 | `200` 更新后活动对象 |
| `DELETE /api/v1/campaigns/{campaignId}` | 无 | `204`，空响应体 |
| `POST /api/v1/campaigns/{campaignId}/treatments` | 不含 ID 的投放项字段 | `201` 投放项对象 |
| `PUT /api/v1/campaigns/{campaignId}/treatments/{treatmentId}` | 完整投放项对象 | `200` 更新后投放项对象 |
| `DELETE /api/v1/campaigns/{campaignId}/treatments/{treatmentId}` | 无 | `204`，空响应体 |
| `GET /api/v1/actions` | 无 | `200 {"actions":[…]}` |
| `GET /api/v1/audience-references` | 无 | `200 {"audienceReferences":[…]}` |
| `GET /api/v1/audience-reference-usages?type=group&groupId=…` | 普通组身份 | `200 {"usages":[…]}` |
| `GET /api/v1/audience-reference-usages?type=arm&bundleId=…&groupId=…` | 包臂身份 | `200 {"usages":[…]}` |

动作目录元素：`{ "id", "name", "requiresSlot": true|false, "available": true|false }`。人群目录元素：`{ "type", "groupId", "bundleId"?, "name", "bundleName"?, "available" }`。占用元素：`{ "campaignId", "campaignName", "treatmentId", "treatmentName" }`；空占用为 `[]`。查询只扫描服务端当前保存的活动，保存、改引、删除成功后立即变化；普通组查询不得带 `bundleId`，包臂查询必须带 `bundleId`。

## 服务端目录与校验

- 构造动作 ID：`action-rcs`、`action-email`、`action-sms` 为推送；`action-show-image` 是需展示位的展示动作；`action-retired` 为不可用反例。
- 构造普通组：`group-new`、`group-recall` 可用，`group-removed` 不可用。已发布包 `bundle-warmup` 的 `group-arm-a`、`group-arm-b`、`group-arm-control` 可用；`bundle-retired/group-arm-retired` 不可用。
- 有效展示位 ID：`slot-home-banner`、`slot-home-hero`。展示动作必须提供其中一个；推送及其他非展示动作不得提供展示位。
- 每个投放项恰好引用一个已知且可用动作、一个已知且可用人群。未知或不可用引用、缺字段、重复投放项 ID 均拒绝。请求不接受未定义字段，因此不能传入多动作列表。
- 同一活动中多个展示投放项占同一展示位且无决策表时，仅同 `bundleId`、不同 `groupId` 的包臂可证互斥；其他组合拒绝。推送不参与展示位竞争。本 V1 不提供决策表编辑。
- 活动流程边端点必须在本活动，且不能指向自己；边 ID 和条件非空且边 ID 不重复。删除被任一现有边引用的投放项返回冲突，保留原状态。

初始活动种子为 `campaign-branch`、`campaign-single`、`campaign-empty`、`campaign-no-edges`，与前端只读演示的常用 ID 对齐。浏览器中 #68 已编辑的受众如何进入服务端目录，本期**未解决**；目录只取服务端构造种子。`audience-reference-usages` 仅报告本服务当前活动的占用，**不能强制阻止浏览器侧删除受众**。

## 错误语义

非 `204` 响应均为 JSON。错误统一为 `{ "error": { "code": "…", "message": "中文说明" } }`。失败写入不修改已保存的活动或投放项。

| HTTP 状态 | 典型 `code` | 含义 |
|---|---|---|
| `400` | `invalid_json`, `missing_field`, `invalid_audience_ref`, `invalid_query` | JSON、字段或查询身份不合法 |
| `404` | `not_found` | 活动、所属投放项或路由不存在 |
| `405` | `method_not_allowed` | 路由存在但方法不支持 |
| `409` | `id_conflict`, `duplicate_treatment`, `treatment_set_conflict`, `flow_read_only`, `slot_conflict`, `edge_conflict`, `invalid_edge`, `duplicate_edge` | ID、投放项集合、活动流程或同位竞争冲突 |
| `415` | `unsupported_media_type` | 写入请求不是 JSON |
| `422` | `unknown_action`, `unavailable_action`, `unknown_audience`, `unavailable_audience`, `missing_slot`, `unknown_slot`, `unexpected_slot` | 引用或展示位不满足目录与动作契约 |

## 验证与边界

`management/internal/httpapi/http_test.go` 用连续 HTTP 请求检验增删改查、进程状态、重新构造时的种子重置、服务端 ID、普通组与包臂、可用性、展示位、同位互斥、活动流程边以及占用变化。当前没有前端接线、浏览器同步、生产存储、真实渠道、执行或发布能力；删除活动会连同其投放项和边一起删除。上线前需另行定义鉴权、持久化、多实例一致性及迁移方案。
