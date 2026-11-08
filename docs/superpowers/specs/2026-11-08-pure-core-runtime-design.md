# 纯化 gmp-core 并迁出状态编排：设计稿

关联 Issue #90；后续细化自 Issue #88。本文是实施前设计稿，尚未授权开始代码实现。

## 意图与原因

`gmp-core` 的目标是承载跨子项目复用的业务值、规则、校验、确定性决策与状态转换，不拥有运行态存储，也不执行外部副作用。当前 Core 仍有内存 Feed / Store / Log / Deliverer、可变 Service 注册表和 `campaign.Runner`；后者还负责调用来源、写执行日志、投递渠道、发布事件与沿回执边推进。因此仅将一个内存 Store 换成 MySQL，仍不能让 Core 成为纯业务模型。

本设计将同一业务语义拆成两个职责：Core 接受显式业务值，返回决策或转换结果；外层应用加载值、调用 Core、提交 MySQL、协调并发 / 幂等 / 重试，并执行事件与渠道副作用。用户已确认接受删除现有 Core Go API 的破坏性迁移，所有仓库内调用方随本任务迁走，不在 Core 留兼容包装。

## 目标

- `gmp-core` 只公开不可变领域值、构造校验、纯规则、确定性决策和纯状态转换；其结果不依赖隐藏的进程状态。
- 受众、实验、活动当前的有状态服务、Store / Feed / Log / Deliverer / EventPublisher 端口与 Runner 移至 Core 外的应用层 / 适配器。
- Core 不使用内存测试存储模拟运行态；Core 业务回归直接给值并断言结果。持久化、重启、事务与并发性质由外层 MySQL 集成用例证明。
- 保留已确认业务语义和现有管理 HTTP `/api/v1` 契约。
- 用类型、包边界和 CI 架构检查阻止状态存储与副作用编排回到 Core。

## 非目标

- 不在本任务内完成 Issue #88 的全部实体 CRUD、生产用户名单访问、生产部署、真实渠道接入或授权体系。
- 不引入 Outbox、投递崩溃恢复、历史内存数据回填、实验指标或未确认业务事实。
- 不更改活动流程边、决策表、分桶算法 / 盐 / 版本、幂等键或受众匹配语义。
- 不以“测试便利”为由在 Core 或 Core 场景中保留 Memory* runtime backend。外层测试可用只记录外部效果的 spy 验证调用，但不能拿它替代 MySQL 持久化验证。

## 硬约束

1. Core 函数只根据显式值与纯算法实现求值；当前状态以值输入，并返回新值 / 业务结果。Core 不加载、保存或修改运行态。
2. Store、Feed、Log、Deliverer、事件发布 / 调度接口属于外层应用；Core 不含其接口、实现、适配器或包装 Service / Runner。
3. 时间由调用方传入；事件、回执、投递动作由 Core 表达为值或转换意图，外层决定何时持久化和执行。
4. 染色输入 / 输出由 Core 的纯分配函数表达；外层读取既有记录，调用纯分配，并通过 MySQL 唯一键与事务原子选定首次记录。为承接现有语义，需要新增平台自有染色记录表；只做向前新增迁移，不从进程内存回填。
5. 用户于 2026-11-08 接受 `gmp-core` 当前 Service / Runner / Memory Go API 的破坏性迁移：所有仓库内调用方同任务迁移，CHANGELOG 列出影响，不在 Core 留兼容 API。管理 HTTP `/api/v1` 不变。
6. 回执仍须由存储原子区分首次接受 / 相同重放 / 冲突；只有首次接受者能发布对应事件并产生回执边的后续意图。Outbox 不在本设计范围。

## 方案

| 方案 | 做法 | 取舍 | 结论 |
|---|---|---|---|
| A. 纯 Core + 外层应用（选用） | 删除 Core 的状态服务、Store / Feed / Log / 外部效果端口与 Runner；将其职责迁至 `management/internal/application` 与 MySQL / 外部效果适配器；Core 改为纯值函数和转换。 | 一次迁移触及三个 Core 层、场景、命令行 Demo 和 management；Go API 破坏性变更，但最终边界完整，能用真实 MySQL 契约验证应用层。 | 用户已确认接受 API 破坏，并要求完成纯 Core 边界。 |
| B. 暂留 Core 兼容包装 | 新应用先搬走，Core 内保留弃用 Service / Runner 与其旧行为一段兼容期。 | 对外迁移较缓，但 Core 仍持有状态与编排，违背本次目标；也会迫使系统维护两条行为路径。 | 否决。用户明确接受破坏性迁移，不要求兼容层。 |
| C. 只删 Memory* | 移除内存具体类型，保留 Core 的 Store / Feed / Log 接口、注册表 Service 和 Runner。 | 改动较小，但 Core 仍定义持久化端口并编排外部副作用，无法满足纯业务边界。 | 否决。 |

采用 A。应用层放在 `management/internal/application`；管理 MySQL 适配器继续留在 Core 外。Core 规则接受显式状态值，返回决策 / 状态转换结果；应用用这些结果协调读写与效果。具体函数签名由实施计划定义，但不得把可变 Registry、Store、Feed、Log 或运行态对象注回 Core。

## 责任映射

| 当前 Core 职责 | 目标位置 | Core 保留什么 |
|---|---|---|
| `audience.Service` 注册来源 / Feed / Bundle / Group 并枚举名单 | 外层应用读取来源和配置；MySQL / 来源适配器提供值 | 单条记录匹配、显式记录集合的筛选 / 选择纯函数；实验份号作为显式事实输入，不从 Service 查询 |
| `experimentation.Service` 持有 ColoringStore、AlgorithmRegistry 与运行中的组件注册表 | 外层应用加载分流运行配置、读取染色记录并通过 MySQL 原子保存 | 给定 Definition、Key 和算法实现的确定性分配；若给定旧染色记录则保留其结果；Component 的校验和状态转换纯函数 |
| `campaign.Service` 持有 slot / campaign 注册表 | 应用加载候选配置并作为输入传入 | 给定候选活动、slot 和事实后的纯路由 / 决策函数 |
| `campaign.Runner`、Log / ReceiptAcceptor、Deliverer、EventSink | 外层应用通过 MySQL 与外部适配器协调 | Execution / Receipt / Event 等领域值；输入已有值、回执或事件后返回下一状态及外部效果意图 |
| `MemoryFeed`、`MemoryColoringStore`、`MemoryLog`、`MemoryDeliverer` | 从 Core 删除；不作为 Core 场景或运行时后端迁移 | 无内存后端 |
| `gmp-core/cmd/gmp-demo` 全流程内存装配 | 移除内存式 runtime 演练；保留 Core 示例时只展示纯值转换，真实运行演示放到外层且要求 MySQL 配置 | 纯 Core API 示例 |
| Core 场景中的端到端 Service / Runner 装配 | 改为纯值输入 / 输出业务回归；应用、MySQL、重启和并发用例放 `management` | 跨 Core 子域的业务不变量，仍随 `gmp-core/scenarios` 在 CI 执行 |

这里的“纯”不是禁止普通 map / slice 值作为本次求值输入；它禁止的是 Core 持有可变的跨调用运行态、外部存储端口与副作用所有权。不能靠一次构造 Service 后持续修改的模式来表达业务。

## 兼容和数据策略

- `gmp-core` 是仓库内公开 Go module；删除导出类型 / 方法属于 source-breaking change。用户批准无 Core API 兼容期和兼容包装，CHANGELOG 使用 `!` 类破坏性记录并列出迁移位置。仓库没有现有 release 或版本 tag；若后续准备发布，发布方仍须将该 breaking change 显式呈现。
- 管理 `/api/v1` 不改路由、JSON 结构或 HTTP 状态码。
- 新增染色表以 `(assignment_id, assignment_key)` 唯一；首次插入选定染色值，冲突时重读获胜值。当前 Core 内存记录不跨进程存活，不做回填；执行 / 回执表结构保持既有形状。新迁移只加表，不改已有记录含义。
- 本地 / CI 只允许构造 MySQL 数据。生产用户名单、数据库主机、凭据和真实渠道均不连接。

## 验证设计

- Core 单元 / 场景测试仅使用显式构造值，验证分桶稳定 / 放量单调、染色重用输入、受众互斥与匹配、决策顺序 / 兜底，以及 Execution / Receipt 状态转换。
- MySQL 集成验证新增染色唯一性与并发首次写胜、执行 / 回执事务、完全相同回执竞争只发布一次事件并只生成一次下一步意图、重启后状态可读。
- 当前已建立五个业务场景继续由 Core `scenarios/` 的纯回归或对应外层集成回归覆盖；不把未建立的场景冒称已覆盖。
- management `/api/v1` 兼容测试保持通过；两个 Go module 执行测试、构建、`go vet`、格式检查；文档一致性与 Core 禁止运行态结构的架构门禁在 CI 通过。
- 验收时对 `gmp-core` 做代码搜索 / AST 架构检查，确保没有 Service/Runner/Store/Feed/Log/Deliverer/EventSink 运行时 API、同步原语承载的跨调用状态或被迁移后的内存适配器；评估器审查范围包括测试与 Demo。

## 风险与边界

- 真风险（阻断）：若状态转换提取漏掉回执关联、execution key、UID / placements 或条件分支，重构会改变投放对象或边目标；必须用原业务场景建立精确结果断言。
- 真风险（阻断）：若并发染色只靠应用进程锁，多个实例可能跳桶；必须由 MySQL 唯一键和并发集成用例证明。
- 真风险（放行后续跟进）：删除 Go API 会影响仓库外 importer；用户接受破坏性迁移，文档提供迁移说明；仓库外使用者无法由当前代码搜索穷尽。
- 真风险（放行后续跟进）：无 Outbox 时提交后发布失败或进程崩溃可能留下未完成的外部效果；本任务不声称自动恢复。
- 伪风险：纯 Core 仍能表达业务状态变化；显式状态值输入与状态转换结果不是 Core 持有存储。
- 未讨论项（评估器攻击点）：纯规则 API 是否遗漏任何隐藏运行态依赖；Algorithm 版本解析是否仍经可变 Registry；应用层的数据库重读是否保持首次写胜；状态转换结果是否完整保留所有原场景的边意图。

## 实施阶段未决给 Builder

- 精确函数签名与值类型布局，须从既有类型 / 规格推导；不得改变语义或引入默认值。
- 适配当前管理 MySQL Store 的端口与类型映射，保持 Core 不依赖 management；若必须改变已有 schema 含义，停下并另行确认，不扩大本设计。
- 用 AST / 包目录规则固化“无状态运行时”时，检查必须自带空源、删包或删类型一类变异用例，防止发现驱动检查静默通过。
