# 纯化 gmp-core 并迁出状态编排实施计划

> **面向代理执行者：** 必须逐任务执行本计划。执行方式由人工选择：`superpowers:subagent-driven-development` 或 `superpowers:executing-plans`。步骤用 checkbox 跟踪。

**目标：** 让 `gmp-core` 只接受显式业务值并返回规则结果或状态转换；由 `management` 应用层通过同一 MySQL 连接池读取名单、保存染色 / 执行 / 回执，并协调投递与事件。

**架构：** 从 Core 删除可变 Service、运行态 Store / Feed / Log 接口、Memory* 后端和 Runner。Core 暴露纯分配、人群判断、活动决策与回执转换；`management/internal/application` 负责串接这些值函数，`management/internal/management` 负责 MySQL。染色用新增的 MySQL 表原子首次写胜，不做回填；管理 `/api/v1` 不变。

**技术栈：** Go 1.24.0；MySQL 8.4；两个独立 Go module；现有 `github.com/go-sql-driver/mysql`。

**规格：** [`docs/superpowers/specs/2026-11-08-pure-core-runtime-design.md`](../specs/2026-11-08-pure-core-runtime-design.md)，任务卡 [`docs/ai-loop/task-cards/90-pure-core-runtime.md`](../../ai-loop/task-cards/90-pure-core-runtime.md)，Issue [#90](https://github.com/snack8310/gmp/issues/90)。

## 全局约束

- Core 规则只读取本次调用显式传入的值，返回值或错误；不得读写运行态、调用数据库、名单来源、渠道、事件或时钟。
- `gmp-core/experimentation` 保持零上层依赖；`audience` 只依赖 `experimentation`；`campaign` 只依赖 `audience`；`scenarios` 只组装纯模型。
- 删除的 Core 导出 API 不保留兼容包装；在 `CHANGELOG.md` 写明 Go source-breaking 影响与外层新入口。
- `/api/v1` 的路径、JSON 字段和状态码保持原样；平台写入范围只含既有执行 / 回执表与新增染色表，不迁移、不回填历史数据。
- 执行 key、UID、treatment、placements 与 receipt outcome 的既有含义保持不变；首次有效回执触发一次事件和一次后续意图，相同回执重放不再触发，冲突回执拒绝。
- MySQL 染色键按 `(assignment_id, assignment_key)` 唯一；同一键并发时以数据库首次写入记录为准，不以进程锁选胜者。
- 所有示例、测试数据均为构造数据；对 MySQL 的读写须有结构化日志，禁止日志输出 DSN、凭据、UID、名单内容或回执敏感属性。
- 不增加除染色表外的持久化结构；组件状态作为显式 Core 值输入 / 输出，由外层调用方提供，本任务不新增实验组件 CRUD。

## Review Focus

1. 同一执行 key 的两个并发相同回执：仅首次 `AcceptReceipt` 的调用能发布事件并运行后续意图。测试：`TestRuntimeConcurrentReceiptEmitsAndFollowsOnce`（Task 4）。
2. 同一 key 的冲突回执：拒绝且不覆盖已接受的 execution / receipt。测试：`TestApplyReceiptRejectsConflictingOutcome`（Task 3）和 MySQL 适配器现有冲突断言（Task 4）。
3. 染色首次写发生并发争用：所有调用读到同一个 MySQL 获胜记录，不同 assignment / key 不串色。测试：`TestMySQLColoringRecordFirstConcurrentWinner`（Task 4）。
4. 演练遇到既有染色时沿用旧份号，遇到新 key 时不写表。测试：`TestRuntimeRehearsalReadsColoringWithoutWriting`（Task 4）。
5. 回执边的下游意图必须携带上游 UID、placements，并以同一上游 execution key 派生幂等 key；目标人群和 rollout 不得再次过滤。测试：`TestApplyReceiptPreservesPersonAndPlacementOnEdge`（Task 3）。

---

## 文件映射

| 位置 | 职责 |
|---|---|
| `gmp-core/experimentation/` | 保留 Definition、Algorithm、Split、Bucket、ColoringRecord 值；新增显式 ComponentState 转换与染色计算；删除 Service、Registry、ColoringStore / MemoryColoringStore。 |
| `gmp-core/audience/` | 保留 Source 描述、Record、Definition、Condition、Group、Bundle、Selection 值；匹配与落点读取 AssignmentFacts；删除 Feed、MemoryFeed、Service。 |
| `gmp-core/campaign/` | 保留活动 / 投放项 / 流程 / 执行 / 回执 / 事件值；新增批次意图与纯转换；删除 Service、Runner、Log、ReceiptAcceptor、Deliverer、EventSink 和 MemoryDeliverer。 |
| `gmp-core/scenarios/` | 把五个已建立业务场景改为显式值输入输出回归，不装配存储、来源接口或 Runner。 |
| `management/internal/application/` | 新建应用协调器；将名单读取、算法解析、染色读写、执行重试、回执原子接受、事件发布和后续意图运行串接起来。 |
| `management/internal/management/` | 新增 MySQL 染色适配器与 migration 003；沿用已有用户池只读、execution / receipt 表和 connection pool。 |
| `management/cmd/gmp-demo/` | 新增依赖本地 MySQL 的真实运行 Demo；`gmp-core/cmd/gmp-demo/` 改为纯值示例或删除并更新根 README。 |
| `gmp-core/internal/arch/`、`.github/workflows/gates.yml` | 加入 Core 运行态 API 禁入架构门禁与空源 / 删除类型变异测试，并在 CI 运行管理应用 MySQL 测试。 |
| `CHANGELOG.md`、`README.md`、`gmp-core/README.md`、`docs/history/changes/` | 记载 breaking Go API 迁移、运行方式、MySQL 读写面、验证结果和未覆盖项。 |

## Task 1：把实验分配和组件生命周期变成显式纯值

**文件：** 修改 `gmp-core/experimentation/assignment.go`、`component.go`、`service.go`、`algorithm.go`、`coloring.go` 与对应测试；新增或更新 `pure_assignment_test.go`、`component_test.go`、`coloring_test.go`。

**接口：**

- 提供字段私有的 `ComponentState` 值及只读访问器；构造与转换方法返回新值，不改输入值或暴露内部 whitelist map。
- 提供 `NewComponentState(Definition) (ComponentState, error)`、`(ComponentState).Start() (ComponentState, error)`、`Stop() (ComponentState, error)`、`Revise(Definition) (ComponentState, error)`、`UpdateSplit(Split) (ComponentState, error)`、`UpdateWhitelist(map[AssignmentKey]BucketNumber) (ComponentState, error)` 与 `Active(Definition, uint64) bool`。
- 提供 `type AssignmentResult struct { Bucket BucketNumber; Coloring ColoringRecord; NeedsPersist bool }` 和 `ResolveAssignment(Definition, AssignmentKey, Algorithm, *ComponentState, *ColoringRecord) (AssignmentResult, error)`。已有 coloring 直接返回其 Bucket 且 `NeedsPersist=false`；无旧记录时用有效 whitelist 或显式 Split / 算法计算；仅彩色定义返回 `NeedsPersist=true` 的新 coloring 值。
- 保留纯 `Algorithm`、`FNV1aV1`、`Definition`、`ColoringRecord` 与 `Calculate`；算法版本选择由外层提供单个 Algorithm，不在 Core 留可变 Registry。

- [x] **Step 1：先写纯计算与状态转换测试。** 新增 `TestResolveAssignmentUsesExistingColoring`、`TestResolveAssignmentReturnsColoringIntent`、`TestComponentTransitionsReturnNewState`、`TestComponentRejectsInvalidSplitAndWhitelist`；断言旧值不变、未知算法版本失败、旧 coloring 胜过新算法结果、停用后 generation 前进。
- [x] **Step 2：运行定向测试确认新接口尚不存在而失败。**

  运行：`cd gmp-core && go test ./experimentation -run 'Test(ResolveAssignment|ComponentTransitions|ComponentRejects)' -count=1`

  预期：编译失败，错误指出 `ComponentState` / `ResolveAssignment` 未定义。
- [x] **Step 3：实现上述纯接口。** 将 component 操作改成值接收者，更新 whitelist 时复制 map；`ResolveAssignment` 校验 Definition、key、算法版本与 component revision / active 状态，不查找或保存 coloring。
- [x] **Step 4：为新纯接口迁移直接分配测试。** 旧 Service 测试暂作既有语义对照；其 API 在 Task 5 完成所有仓内调用迁移后统一删除，不提供兼容包装。
- [x] **Step 5：验证实验包。**

  运行：`cd gmp-core && go test ./experimentation && go vet ./experimentation`

  预期：通过；新值转换测试通过，旧 Service 仅为尚未迁移的调用方暂留，最终 API 删除验收在 Task 5。
- [x] **Step 6：提交本任务。** `git add gmp-core/experimentation && git commit -m "refactor(experimentation): add pure component transitions"`。

## Task 2：将人群匹配改为显式记录与份号事实

**文件：** 修改 `gmp-core/audience/condition.go`、`definition.go`、`group.go`、`placement.go`、`selection.go`、`source.go`、`service.go`、`memory.go` 及对应单测。

**接口：**

- 提供 `type AssignmentFact struct { Definition experimentation.Definition; Bucket experimentation.BucketNumber }` 与 `type AssignmentFacts []AssignmentFact`；查找按 Definition 的完整 revision 比较，避免同一 ID 的不同 revision 被压成一个事实。
- 提供 `func (Definition) Matches(Record, AssignmentFacts) (bool, error)` 与 `func (Definition) Placements(UID, AssignmentFacts) ([]Placement, error)`。
- 提供 `func (UID) AssignmentKey() experimentation.AssignmentKey`，供外层为同一个 UID 解析实验事实。
- 提供 `func (Group) Matches(Record, AssignmentFacts) (bool, error)` 与 `func (Selection) Match(Record, AssignmentFacts) (Group, bool, error)`；Selection / Bundle 的构造校验只检查显式结构，不读注册表或共享状态。
- 提供 `func (Definition) Assignments() []experimentation.Definition`，按首次出现顺序去重列出条件涉及的 Assignment；不暴露可变内部切片。

- [x] **Step 1：新增纯人群测试。** 新增 `TestDefinitionMatchesUsingExplicitAssignmentFacts`、`TestDefinitionReportsPlacementsFromFacts`、`TestSelectionMatchesExactlyOneBundleArm`、`TestMissingAssignmentFactFailsClosed`；断言同一记录和事实下匹配 / 放置确定，缺少份号报错，枚举无需来源接口。
- [x] **Step 2：运行定向测试确认失败。**

  运行：`cd gmp-core && go test ./audience -run 'Test(DefinitionMatchesUsingExplicit|DefinitionReportsPlacements|SelectionMatchesExactly|MissingAssignmentFact)' -count=1`

  预期：接口未定义导致编译失败。
- [x] **Step 3：实现显式 Facts 求值。** `shareCondition` 与 `shareAtMostCondition` 只从 `AssignmentFacts` 读取；禁止它们计算、染色或请求外部服务。枚举等价性由纯 `Definition.Matches` 保障。
- [x] **Step 4：将纯 Group / Selection 求值与 runtime availability 校验分开。** 纯 `Matches` / `Match` 只用定义、记录与 facts；bundle pointer / atomic 生命周期以及旧 Service 校验在 Task 5 调用方迁移时统一改成外层显式值，不在最终 Core 留下。
- [x] **Step 5：迁移直接求值测试到纯函数。** 旧 Source / Feed / MemoryFeed / Service 暂留供 Task 5 的调用方迁移；来源能力最终只留下描述值，通过 `Definition.Matches` 断言属性、窗口、实验臂、互斥与 placement 语义。
- [x] **Step 6：验证人群包。** `cd gmp-core && go test ./audience && go vet ./audience`；预期新纯函数和既有对照测试通过，旧 Feed / Service 暂留到 Task 5。
- [x] **Step 7：提交本任务。** `git add gmp-core/audience && git commit -m "refactor(audience): evaluate explicit audience facts"`。

## Task 3：把活动决策与执行 / 回执转换变成纯函数

**文件：** 修改 `gmp-core/campaign/service.go`、`runner.go`、`execution.go`、`delivery.go`、`event.go`、`memorydeliverer.go`、`campaign.go` 与对应测试。

**接口：**

- 提供 `func Decide(campaigns []Campaign, slots []Slot, slot SlotID, record audience.Record, facts audience.AssignmentFacts) (Decision, error)`；候选顺序仍由现有 priority 规则决定，priority tie 仍报 `ErrContentionUndecidable`。
- 提供 `type DeliveryIntent struct { Key IdempotencyKey; UID audience.UID; Campaign CampaignID; Treatment TreatmentID; Action ActionID; Call Call; Placements []audience.Placement }`。
- 提供 `type Report struct { Campaign CampaignID; Treatment TreatmentID; Rehearsal bool; Planned []Plan; Executions []Execution }`，将现有 Runner 报告值留在 Core；`Plan` 保存 Key / UID / Action / Placements，不持有执行状态。
- 提供 `func PlanEntry(Campaign, TreatmentID, audience.Record, audience.AssignmentFacts, Occurrence) (DeliveryIntent, bool, error)`，只表达应投递项，不枚举来源、不重试、不保存或执行投递。
- 提供 `func CompleteDelivery(DeliveryIntent, attempts int, ack Ack, succeeded bool) (Execution, error)`，在值层构造 Delivered / Failed Execution。
- 提供 `type ReceiptTransition struct { Execution Execution; Event Event; Deliveries []DeliveryIntent; Noop bool }` 和 `func ApplyReceipt(Campaign, Execution, Receipt) (ReceiptTransition, error)`；仅生成结果值，不承诺存储原子性；应用层先由 `AcceptReceipt` 决定是否执行 Event / Deliveries。

- [x] **Step 1：新增纯决策与转换回归。** 新增 `TestDecideUsesExplicitFactsAndPriority`、`TestPlanEntryIncludesPlacementsAndDerivedKey`、`TestCompleteDeliveryBuildsExecutionFromIntent`、`TestApplyReceiptPreservesPersonAndPlacementOnEdge`、`TestApplyReceiptRejectsConflictingOutcome`、`TestApplyReceiptCopiesInputPlacements`；receipt replay 产生 `Noop` 且无 deliveries；回执边只产生符合 outcome 的一步意图。
- [x] **Step 2：运行定向测试确认新接口失败。** `cd gmp-core && go test ./campaign -run 'Test(DecideUsesExplicit|PlanEntryIncludes|ApplyReceiptPreserves|ApplyReceiptRejects)' -count=1`；预期接口未定义。
- [x] **Step 3：实现纯决策、投递计划、delivery 完成与 receipt 转换。** 下游 key 继续由 campaign、treatment、UID 与上游 execution key 派生；目标 audience / rollout 不再重复筛边上 UID；复制 Placements 和 receipt attributes，避免结果共享可变输入。
- [x] **Step 4：迁移 Campaign service 测试到 `Decide` 参数列表；迁移 Runner 执行 / 回执测试到纯转换测试。** 保留 campaign 构造、slot 优先级、决策表顺序 / 兜底 / 互斥与流程校验。
- [x] **Step 5：迁移直接 execution / receipt 测试到纯转换。** Runner / Log / ReceiptAcceptor / Deliverer / EventSink / DiscardEvents / MemoryDeliverer 暂留供 Task 5 的场景与 Demo 迁移；最终删除时不留兼容包装。
- [x] **Step 6：验证 campaign 包。** `cd gmp-core && go test ./campaign && go vet ./campaign`；预期纯决策与回执值转换测试通过，包内没有同步 / I/O / 外部调用。
- [x] **Step 7：提交本任务。** `git add gmp-core/campaign && git commit -m "refactor(campaign): add pure execution transitions"`。

## Task 4：在外层应用接回 MySQL、来源、执行、回执与副作用

**文件：** 新增 `management/internal/application/runtime.go`、`audience.go`、`delivery.go`、相应测试；修改 `management/internal/management/mysql_schema.go`、`mysql_store.go`；新增 `migrations/003_experiment_colorings.sql`、`mysql_coloring_store.go` 及 MySQL 集成测试。

**接口：**

- 应用包提供 `type Deliverer interface { Deliver(context.Context, campaign.Request) (campaign.Ack, error) }` 与 `type EventPublisher interface { Publish(context.Context, campaign.Event) error }`。
- 应用包 `Runtime` 接受显式 `RuntimeSpec{Store *managementstore.MySQLStore, Source *managementstore.MySQLAudienceSource, SourceID audience.SourceID, Algorithms map[experimentation.AlgorithmVersion]experimentation.Algorithm, Components map[experimentation.AssignmentID]experimentation.ComponentState, Deliverers map[campaign.ActionID]Deliverer, Events EventPublisher, MaxAttempts int}`；构造 `NewRuntime(RuntimeSpec) (*Runtime, error)` 校验 nil / 缺项；不设置隐式重试、分页或算法默认值。
- 应用 `Run(ctx context.Context, built campaign.Campaign, treatment campaign.TreatmentID, occurrence campaign.Occurrence, mode audience.Mode) (campaign.Report, error)` 遍历只读 source pages，按 Definition 条件顺序惰性解析 `AssignmentFacts`（使用 `NextMissingAssignment` 与 UID.AssignmentKey），读取旧 coloring，调用纯 Core，再按 mode 选择是否调用 MySQL `RecordColoringFirst`；静态前置条件未命中时不得提前染色。执行每个 `DeliveryIntent` 时用相同 key 重试并写入 ExecutionLog。
- 应用 `RecordReceipt(ctx context.Context, campaign.Campaign, campaign.Receipt) error` 先读取 execution、调用 `campaign.ApplyReceipt`，在 MySQL 原子接受首次 receipt 后才发布 Event、执行 `Deliveries`；相同 receipt 重放返回成功但不重复副作用，冲突失败。
- MySQL coloring adapter 暴露 `LookupColoring(ctx, AssignmentID, AssignmentKey) (ColoringRecord, bool, error)` 与 `RecordColoringFirst(ctx, AssignmentID, AssignmentKey, ColoringRecord) (ColoringRecord, error)`；表用 `UNIQUE(assignment_hash, key_hash)`，分别存两值的 SHA-256 与原始 assignment / key 供碰撞检测；冲突时重读获胜行。
- 应用层 Deliverer / EventPublisher 接口定义在 `management/internal/application`，不是 `gmp-core`。

- [x] **Step 1：新增 MySQL coloring 集成测试。** `TestMySQLColoringRecordFirstConcurrentWinner` 并发写同一 key 的不同 bucket，断言所有调用返回同一行；再断言不同 assignment / key 的记录互不覆盖。
- [x] **Step 2：运行该测试确认迁移和 adapter 不存在时失败。** `cd management && GMP_TEST_MYSQL_DSN='root@tcp(127.0.0.1:3306)/gmp_test?parseTime=true' go test ./internal/management -run TestMySQLColoringRecordFirstConcurrentWinner -count=1`；预期编译失败，缺少 MySQL coloring adapter。
- [x] **Step 3：新增 migration 003 与 schema contract。** 新表使用 InnoDB、哈希唯一索引及原值列；更新 migration runner 与 schema verifier；不得改写 001 / 002 或已有执行数据。
- [x] **Step 4：实现 MySQL coloring adapter。** 对冲突插入重读数据库获胜行；核对原值防止 SHA-256 key collision；所有 DB 操作延用现有结构化日志 helper，不打印原始 key / DSN / 个人数据。
- [x] **Step 5：实现并验证 adapter。** 运行：`cd management && GMP_TEST_MYSQL_DSN='root@tcp(127.0.0.1:3306)/gmp_test?parseTime=true' go test ./internal/management -run 'TestMySQLColoring' -count=1`；预期并发首写唯一性测试通过。
- [x] **Step 6：验证 Runtime 的 receipt 原子性与染色时点。** `TestRuntimeConcurrentReceiptEmitsAndFollowsOnce` 使用两个 MySQLStore 实例、构造名单和只记录外部效果的 test spy；断言事件数、下一步调用数各为 1，UID / placements / key 完全相等；`TestRuntimeRehearsalReadsColoringWithoutWriting` 断言演练只读；`TestRuntimeDoesNotColorMemberRejectedBeforeAssignment` 断言条件顺序中更早失败的属性条件不会触发染色。
- [x] **Step 7：实现 Runtime 编排。** 复用管理 Store 的执行 / 回执 MySQL 实现与名单 source，不在应用层建内存执行态库；MySQL `AcceptReceipt` 获胜后才执行 event / downstream intent。
- [x] **Step 8：运行 Runtime / MySQL 集成测试。** `cd management && GMP_TEST_MYSQL_DSN='root@tcp(127.0.0.1:3306)/gmp_test?parseTime=true' go test ./internal/application ./internal/management -count=1`；预期重启后执行与回执可读、冲突不改值、并发回执只产生一次效果。
- [x] **Step 9：提交本任务。** `git add management && git commit -m "feat(management): run pure core through mysql application"`。

## Task 5：迁移业务场景与演示入口

**文件：** 重写 `gmp-core/scenarios/` 和 `gmp-core/cmd/gmp-demo/`；新增 `management/cmd/gmp-demo/` 及 `management/internal/application/demo.go`；更新 `gmp-core/README.md`、根 `README.md`。

**接口：**

- `gmp-core/scenarios` 仅用 Core 值和纯函数构造断言，不暴露 `Stack`、runtime service 或内存后端。
- `management/cmd/gmp-demo` 从必填 MySQL 环境与现有构造用户池读取样本，使用 Task 4 的 Runtime / MySQL 记录执行及回执；副作用只能由本地构造 Deliverer / EventPublisher 收集。
- `gmp-core/cmd/gmp-demo` 仅展示纯 `Definition.Matches`、`Calculate`、`PlanEntry` / `ApplyReceipt` 值转换，不创建 Fake Store / Source / Runner；若不再提供有意义内容，则删除并从 README / CI 移除该入口。

- [x] **Step 1：将已有五个业务场景逐个列为纯场景回归输入输出。** 每个已建立场景保留同一核心断言：新客首单已建立部分、双十一四臂与对照、banner 抢位 / 决策表、灰度单调性、跨境两级回流时间窗与回执边 UID / key；对当前明确未建立的内容仍记为未建立。
- [x] **Step 2：运行 `cd gmp-core && go test ./scenarios -count=1` 确认纯场景迁移前的编译失败。** 预期指出已删除的 runtime API 引用。
- [x] **Step 3：重写 `scenarios/` 回归为显式值函数。** 不增加 `未使用`、指标或其他未确认业务事实；根据根 `AGENTS.md` 保持场景覆盖表的已建立 / 未建立状态。
- [x] **Step 3a：迁移管理 API 与场景 / Demo 的所有旧 Core 调用方。** 将 management 集成用例改用 Task 4 Runtime；迁移五个场景与 Demo 至显式纯值 / 外层 MySQL Runtime；仓内 `rg` 不再发现旧 Service / Runner / Memory API 使用点。
- [x] **Step 3b：删除所有 Core Service / Runner / Store / Feed / Log / Deliverer / EventSink、atomic / Service 指针生命周期状态与 Memory* runtime 实现及其专属文件 / 测试。** 用户已接受 breaking；不在 Core 留兼容包装。保留的 test fixture 只构造领域输入，不实现运行态存储。
- [x] **Step 4：先写 MySQL Demo 启动校验测试。** `TestDemoRequiresExplicitMySQLConfig` 断言缺少连接池配置时失败，不输出 DSN；`TestDemoUsesConstructedAudiencePool` 断言只读 `gmp_demo_user_pool` 并写平台执行表。
- [x] **Step 5：实现 management Demo 与 Core 纯值 Demo。** 根 README 将真实运行命令指向 management 并明确依赖 MySQL；Core README 明确 Core 不连接或提供内存存储。
- [x] **Step 6：运行 Demo 与场景。** `cd gmp-core && go test ./scenarios && go run ./cmd/gmp-demo`；`cd management && GMP_TEST_MYSQL_DSN='root@tcp(127.0.0.1:3309)/gmp_test?parseTime=true' go test ./cmd/gmp-demo ./internal/application`。预期两入口退出码为 0，Core Demo 不访问外部资源，management Demo 仅访问构造 MySQL。
- [x] **Step 7：提交本任务。** `git add gmp-core management/cmd management/internal/application/demo.go README.md && git commit -m "refactor(scenarios)!: separate pure and mysql demos"`（commit `4889fc4`）。

## Task 6：固化边界、兼容记录、CI 与交付证据

**文件：** 修改 `gmp-core/internal/arch/arch.go`、`arch_test.go`、`.github/workflows/gates.yml`、`CHANGELOG.md`、`README.md`、`gmp-core/README.md`、`AGENTS.md`、`docs/history/changes/2026-11-08-pure-core-runtime.md`。

- [x] **Step 1：先为架构门禁写测试。** `TestCoreRejectsRuntimeOwnershipTypes` 检查四个 Core 包没有 Service / Runner / Store / Feed / Log / Deliverer / EventSink 类型，且没有 `sync`、`database/sql`、net、日志或直接时钟访问；门禁测试覆盖禁止类型 / import、时钟访问和空源。
- [x] **Step 2：运行架构门禁测试确认失败。** 定向用例先因未拒绝 `time.Now` 和 `log` import 而失败，随后实现门禁。
- [x] **Step 3：实现基于 Go parser 的声明 / import 检查。** 对目录不存在、无 Go 源文件、无可检查声明一律失败；显式运行态类型名清单与禁止依赖 / 时钟调用均有正反断言。
- [x] **Step 4：将门禁纳入现有 `go test ./...` 并核验 CI 管理应用与 MySQL 集成步骤。** 现有 workflow 已提供 MySQL 8.4 service，并在 management module 执行 build、vet、test；测试使用构造库 DSN，不写 secret 到日志。
- [x] **Step 5：记录 breaking 变更。** `CHANGELOG.md` 写明删除的 Go API 类型 / 方法、外层迁移入口、仓内迁移完成与外部 importer 需改用应用层；管理 HTTP 与旧数据结构保持不变。
- [ ] **Step 6：更新文档与变更证据。** README、Core README、领域边界、AGENTS 已说明 Core / 应用 / MySQL 责任、命令、读写面、迁移只新增表且不回填、已建立业务场景覆盖与未覆盖；按仓库记忆时机，历史变更证据须在首次独立评估通过后写入，再交最终评估。
- [x] **Step 7：执行最终验证。** 最新架构门禁和别名保护改动后，已重新执行以下验证：

  运行：

  ```sh
  (cd gmp-core && GOTOOLCHAIN=go1.24.0 go build ./... && GOTOOLCHAIN=go1.24.0 go vet ./... && GOTOOLCHAIN=go1.24.0 go test ./... -count=1 && GOTOOLCHAIN=go1.24.0 go run ./cmd/gmp-demo)
  (cd management && GOTOOLCHAIN=go1.24.0 go build ./... && GOTOOLCHAIN=go1.24.0 go vet ./... && GMP_TEST_MYSQL_DSN='root@tcp(127.0.0.1:3309)/gmp_issue90_verify?parseTime=true' GOTOOLCHAIN=go1.24.0 go test ./... -count=1)
  python3 test/docs/consistency_check.py
  git diff --check
  ```

  结果：`gmp-core` 与 `management` module 的 build、vet、全量测试及 Core Demo 均退出码 0；MySQL 集成测试使用专用本地 schema 后通过，schema 已删除且容器已停止；文档一致性检查、变更 Go 文件格式检查、`git diff --check` 均通过。
- [ ] **Step 8：对实现与架构文档做独立只读评估。** 按协议交付准确 diff、任务卡、设计稿与验证结果；只有 `PASS` 后才写新增历史变更证据。
- [ ] **Step 9：提交最终证据并复审精确 HEAD。** 新增 `docs/history/changes/2026-11-09-pure-core-runtime.md`，填写风险、MySQL 读写面与未覆盖项；提交变更证据后，独立评估器对最终 HEAD 再次只读评估并将 `PASS` 写入 PR 模板正文。
- [ ] **Step 10：推送分支并创建 Draft PR。** 关联 Issue #90，标题 `[Refactor] 纯化 gmp-core 并迁出状态编排`，正文填完整模板与最终评估器 `PASS` 证据；保持 Draft，不合并、不请求最终人工审核。

## 计划自审

- **规格覆盖：** Task 1–3 新增纯模型 / 规则 / 状态转换并暂留迁移期旧 API；Task 4 覆盖名单 MySQL 读、染色 / 执行 / 回执 MySQL 写、原子幂等与外部效果；Task 5 迁移完所有仓内调用后统一删旧 API、覆盖场景与 Demo；Task 6 覆盖破坏性变更、架构门禁、API 兼容、CI 与交付证据。Outbox、生产数据 / 渠道、未确认业务事实均不在实现范围。
- **步骤粒度：** 每个 Task 均按先测失败、实现、验证、提交拆分；接口名及每个验证命令已列出。MySQL 演示只读取构造用户池，不写或回填名单。
- **类型一致性：** `ComponentState` / `ResolveAssignment` 输出的 facts 由 `Definition.Assignments` 与 `UID.AssignmentKey` 供应用生成；campaign 的 `DeliveryIntent` 和 `ReceiptTransition` 是应用执行的唯一动作输入。
- **Review Focus：** 五个重点均有明确测试名和所属任务，覆盖并发首次回执、冲突、染色 first-write-wins、演练只读、回执边上下文。
- **比例：** 计划将大任务分为六个可独立拒收的交付，未罗列每个 helper 的实现细节；最终签名均在消费前定义。
