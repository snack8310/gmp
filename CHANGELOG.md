# 变更记录

## 未发布

### 破坏性变更

- **`gmp-core` Go API 破坏性迁移（Issue #90）**：删除 `audience.Service` / `Feed` / `MemoryFeed` / `Mode`，`experimentation.Service` / `AlgorithmRegistry` / `ColoringStore` / `MemoryColoringStore`，以及 `campaign.Service` / `Runner` / `RunnerSpec` / `Log` / `ReceiptAcceptor` / `Deliverer` / `EventSink` / `DiscardEvents` / `MemoryLog` / `MemoryDeliverer`。名单注册、查询与枚举，实验服务的分配 / peek / 组件注册与启停，以及活动注册 / 抢位服务和 Runner 的批次运行 / 回执入口均删除，不保留兼容包装。删除注册式值构造入口 `audience.RegisterSource`、`campaign.RegisterAction`、`campaign.RegisterSlot`，分别改用 `NewSource`、`NewAction`、`NewSlot`；实验分配改为传入显式算法调用 `ResolveAssignment`，组件生命周期改为 `ComponentState` 值转换。调用方以显式值 API（`Calculate`、`Definition.Matches`、`Selection.Match`、`Decide`、`PlanEntry`、`CompleteDelivery`、`ApplyReceipt`）表达规则和转换；成员查询、染色 / 执行 / 回执持久化、并发仲裁、重试及外部效果由 `management/internal/application` 与 MySQL / 外部适配器处理。删除 `campaign/deliverytest` 和 `experimentation/coloringtest` 导出的实现契约测试包；仓外适配器需自行维护其投递幂等与染色首次写胜回归。运行时专属错误移到对应外层应用或存储适配器。该变更按用户确认不保留 Core 兼容包装；仓外 Go importer 必须迁移。
- 同一变更在 management 新增 MySQL 染色表 migration 003，只新增表、不回填；执行与回执继续写现有平台表。管理 HTTP `/api/v1` 契约与既有执行 / 回执数据格式不变。`gmp-core/cmd/gmp-demo` 现在只演示纯值转换；需要 MySQL 的运行演示移至 `management/cmd/gmp-demo`，名单 SQL 只使用构造数据，投递 / 事件不连接真实渠道。
- 浏览器实验演示缓存 `gmp-audience-demo-v1` 的内部格式从 v4 升到 v5，增加实验比例、是否记录首次分配和构造用户模拟染色。旧版页面不能读取 v5；首次升级写入前会将旧版原文保存到 `gmp-audience-demo-v1-backup-v4`。回退旧页面前须把备份原文恢复到主键；升级后的修改不会自动回迁。影响范围仅为本地演示浏览器缓存，不涉及管理 API 或 Core。
- `management` 活动管理 `/api/v1` 原地调整：活动使用单个 `audience`（普通人群组或实验人群包）；`treatments[]` 移除 `audienceRef`；`activityFlow` 新增 `entries[]` 入口路由；受众与流程分别使用 `/audience`、`/flow` 写入。旧 `audienceRefs[]`、投放项级 `audienceRef`、`/audiences` 和旧流程请求体不再兼容。
- 活动实验臂现在显式路由到不同起始投放项或显式无投放对照；后续投放由活动回执边推进。核心 `gmp-core` 活动要求单一受众及完整活动流程。
- 影响范围：当前仓库内本机演示客户端与活动管理 API。该 V1 尚未合并，用户已确认原地更新、不增加 V2；没有生产调用方、持久化数据或迁移步骤。

