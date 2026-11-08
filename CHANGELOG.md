# 变更记录

## 未发布

### 破坏性变更

- `campaign.NewRunner` 的执行日志除实现 `campaign.Log` 外，还必须实现 `campaign.ReceiptAcceptor`，否则构造时返回 `ErrReceiptAcceptanceUnavailable`。自定义日志适配器须增加原子回执接受：首次写入返回 `true`，相同回执重放返回 `false`，冲突回执返回 `ErrConflictingReceipt`。不提供旧实现兼容回退，因为它无法保证多 Runner 并发时只发布一次事件、只推进一次流程边。此变更尚未合并或发布；迁移方式随本次未发布变更提供，HTTP API、数据库 schema 与存量数据格式不变。
- 浏览器实验演示缓存 `gmp-audience-demo-v1` 的内部格式从 v4 升到 v5，增加实验比例、是否记录首次分配和构造用户模拟染色。旧版页面不能读取 v5；首次升级写入前会将旧版原文保存到 `gmp-audience-demo-v1-backup-v4`。回退旧页面前须把备份原文恢复到主键；升级后的修改不会自动回迁。影响范围仅为本地演示浏览器缓存，不涉及管理 API 或 Core。
- `management` 活动管理 `/api/v1` 原地调整：活动使用单个 `audience`（普通人群组或实验人群包）；`treatments[]` 移除 `audienceRef`；`activityFlow` 新增 `entries[]` 入口路由；受众与流程分别使用 `/audience`、`/flow` 写入。旧 `audienceRefs[]`、投放项级 `audienceRef`、`/audiences` 和旧流程请求体不再兼容。
- 活动实验臂现在显式路由到不同起始投放项或显式无投放对照；后续投放由活动回执边推进。核心 `gmp-core` 活动要求单一受众及完整活动流程。
- 影响范围：当前仓库内本机演示客户端与活动管理 API。该 V1 尚未合并，用户已确认原地更新、不增加 V2；没有生产调用方、持久化数据或迁移步骤。

