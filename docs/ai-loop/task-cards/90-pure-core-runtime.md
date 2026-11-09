# Issue #90：纯化 gmp-core 并迁出状态编排

关联 Issue [#90](https://github.com/snack8310/gmp/issues/90)，后续细化自 [#88](https://github.com/snack8310/gmp/issues/88)。新需求开发，大任务，高风险与对外破坏性变更。

## 任务入口检查

### 分类与问题本质

- 类型：新需求开发 / 架构边界重构。
- 等级：大任务。理由：变更 `gmp-core` 公开 Go API，重画 Core / management 边界，影响受众、实验、活动、场景、Demo 与 MySQL 运行态。
- 用户表面请求：完成 Core 无存储、无内存实现与无状态编排的要求。
- 要解决的真实问题：Core 仍持有可变 Service 注册表、Feed / ColoringStore / Log 等存储端口与 Memory* 后端，Runner 承担投递、事件和回执后的流程副作用；内存端到端演练不能证明外层真实持久化。
- 硬约束：Core 以值输入 / 输出表达规则和状态转换；来源查询、存储、并发、重试、调度与外部效果由外层应用负责；业务语义和管理 HTTP 契约不变。
- 方案选择：拆出纯业务函数与状态转换，将应用服务 / MySQL / 外部效果端口迁至 `management` 外层。用户于 2026-11-08 明确接受删除现有 Core Go API、同步迁移仓库内调用方、CHANGELOG 标注破坏性变更、不保留 Core 兼容包装。

### 范围与非目标

- 范围内：从 `gmp-core` 删除有状态 Service / Runner / Store / Feed / Log / 外部效果接口及 Memory* 实现；将必要编排移到 management 外层；以 MySQL 保存执行 / 回执和染色运行态；纯化 Core 业务场景；更新 Demo、依赖检查、CHANGELOG 与文档。
- 非目标：不完成 Issue #88 全部 CRUD / 管理模型统一，不接生产用户池、真实渠道或生产部署，不做 Outbox / 崩溃恢复或历史数据回填，不改 `/api/v1` 契约；不新增活动流程边上的等待时长，新客首单转化与跨境召回继续通过跨活动回流表达时间窗。
- 影响面：`gmp-core` 公开 Go API 为破坏性变更；`management` 增加 Core 应用编排和染色表迁移；`gmp-core` 与 `management` module 的测试及 Docker / CLI 演示边界需复核。
- 外部读 / 写：本地和 CI 对构造 MySQL 执行平台自有表写入；demo 用户池路径只读；新增染色表只新增结构、不回填；不访问生产数据库、真实用户数据或真实渠道；外部事件 / 投递只由测试 spy 记录。

### 高风险项

| 判定项 | 是否命中 | 依据 |
|---|---|---|
| 通用标准 2：外部 Go API / 数据格式 | 是 | 移除现有 Service / Runner / Memory 导出 API；用户已接受破坏性迁移。 |
| 通用标准 3：幂等、并发、执行顺序 | 是 | 回执接受、边推进、重试与染色首次写胜必须保持。 |
| 通用标准 4：持久化结构 | 是 | 外层增加染色记录表；只加新表、不修改已有数据含义。 |
| 通用标准 6：同能力多实现一致 | 是 | 纯 Core 转换、MySQL 应用实现与测试契约需一致。 |
| 通用标准 7：跨模块影响 | 是 | `gmp-core`、management 应用、MySQL 适配器、HTTP 调用方及演示命令。 |
| 项目清单 #1：平台事件与活动边语义 | 否（保持） | 仅迁移事件发布职责；成功投递仍产生 `DeliveredEvent`，回执仍产生 `ArrivedEvent`，活动边匹配仍按契约回执结果。新客 72 小时窗口继续从成功投递事件起算。 |
| 项目高风险 #3 / #6 / #10 / #16 | 是 | 执行与回执关联、统一执行契约、执行态实现一致性、Core 依赖分层。 |
| 项目高风险 #5：算法版本与染色 | 是 | 染色首次写胜依赖稳定唯一键及算法版本，外层持久化不得造成历史跳桶。 |
| 对外破坏性变更 | 是 | 删除公开 Go API；无兼容 wrapper，CHANGELOG 给出受影响 API 与迁移位置。 |

### 人工确认

- 用户已确认目标为 Core 无数据存储 / 内存 Store，外层负责真实 CRUD 与流程编排（2026-11-08）。
- 用户已确认接受移除 `gmp-core` 当前公开 Service / Runner / Memory API，同步迁移仓库内调用方，并在 CHANGELOG 标注 breaking；不保留兼容包装（2026-11-08）。
- 用户已批准按本任务卡与设计稿实施 Issue #90，确认实施计划准确，并选择 Native 直接实施；交付目标为 Draft PR，禁止 AI 自行合并。
- 持久化边界沿用 Issue #88 和 `docs/domain/system-boundaries.md`：染色记录归外层并原子首次写胜；本任务若需改变已有数据含义则必须停止并重新确认。

### 风险三档

- 真风险（阻断）：当回执转换遗漏 UID、execution key、placements、首次接受或边目标时，重试可能重复触达或错误推进；必须有跨层精确断言和 MySQL 并发用例。
- 真风险（阻断）：当染色唯一性仅由单进程互斥保证时，多实例可能跳桶；必须由数据库唯一约束和并发集成用例证明。
- 真风险（放行后续跟进）：当仓库外 Go importer 继续调用旧 API，会因移除而编译失败；用户已接受 breaking，需通过 CHANGELOG 列迁移路径；仓库内搜索无法穷尽外部消费者。
- 伪风险：纯 Core 不等于无状态转换；把已有状态值传入函数并返回下一状态是规则计算，不是存储。
- 未讨论项（评估器重点）：命名结构体中的导入 map / pointer 或已允许切片是否可经局部取址、接收器拷贝、复合字面量或 helper 参数绕过 Core 门禁，尤其是切片共享底层数组的跨调用修改；是否还存在未迁出的隐式状态 / 注册；Algorithm 版本解析是否仍依赖可变 Registry；新染色唯一约束是否与用户键和算法版本规则一致；所有已建立场景回流与活动内回执边效果是否仍保留。

### 历史规则命中

| 来源 | 适用主题 | 是否命中 | 应用方式 / 不适用原因 |
|---|---|---|---|
| `docs/history/memory/receipt-edge-audience-partition.md` | 回执边身份沿用、派生 execution key、placements 与首次接受者副作用 | 命中 | Core 转换保留 UID、派生 execution key、placements；只有原子回执首次接受者触发副作用。 |
| `docs/history/memory/review-package-consistency.md` | 载体陈述一致、准确送审 SHA 与 CI headSha 对齐、外部写出后回读 | 命中 | Issue、任务卡、设计稿、PR 与准确送审 SHA 保持一致；外部写出后回读确认。 |
| `docs/history/memory/source-enumeration-fidelity.md` | 来源侧逐项遍历、送审摘要不得固定计数 | 命中 | 任务卡逐项枚举当前 Core 服务、接口、内存适配器、Demo / 场景调用方并标记迁移去向；摘要用名称列举，不写固定计数。 |
| `docs/history/memory/mechanization-claims.md` | 约束优先机械化、存量复核、空源与退化变异、断言期望强度 | 命中 | 新约束优先固化；新架构检查复核存量并有空源 / 删除 / 退化变异，测试期望可击穿恒定错误实现。 |
| `docs/history/changes/2026-11-08-concurrent-receipt-acceptance.md` | 关键实现与风险 | 命中 | 迁移后的回执存储仍需原子接受；无 Outbox 的恢复限制继续作为未覆盖。 |
| `docs/history/changes/2026-11-05-gmp-core-readme-status.md` | Core 状态与限制 | 命中 | 区分已存在模型 / 规则与仍有内存实现的部分，不把旧内存行为误认为已完成架构目标。 |
| `docs/history/changes/2026-10-27-campaign-flow-edges.md`、`docs/history/changes/2026-10-28-flow-edge-scenarios.md` | 活动内回执边与跨活动回流等待的边界 | 命中 | 场景①与⑤的等待时长未被定义为边条件；新客三天提醒与跨境四小时召回继续通过回流事件和独立活动表达。 |
| `docs/history/changes/2026-10-29-campaign-management-api.md` | API 契约与进程状态 | 命中 | HTTP API 当前与 Core 解耦；保持其 JSON 契约，并单独报告 Core 应用接线范围。 |
| `docs/domain/system-boundaries.md` | 148–247、B7 | 命中 | Core 纯规则；状态、MySQL、事件与时间由外层管理；场景与 MySQL 集成测试按边界分放。 |
| `docs/domain/business-scenarios.md` | 新客首单转化与覆盖矩阵 | 命中 | 按既有活动边决策澄清新客首单转化通过跨活动回流等待，而不是同活动回执边；同文件矩阵同步区分两种串联方式。 |
| `AGENTS.md` | 高风险清单 #3/#5/#6/#10/#16、业务回归场景、协作流程 | 命中 | 命中高风险；保留已建立场景部分：新客首单转化的发券与三天后回流、双十一分组对照与触达、首页 banner 抢位与决策、新玩法放量单调性、跨境召回两步事件与回执幂等；未建立边界按 AGENTS 记录；issue → branch → PR → 独立评估 PASS → 人工合并。 |

历史扫描方式：逐个读取 `docs/history/memory/` 并从近 30 天变更中筛选 Core / 管理执行边界最相关的记录；实际命中如上。未读记录为用户界面、视觉、Shell 和与状态迁移无关的产品变更，因主题与机制不匹配不纳入本任务证据。

### 最小验证集

- 必跑：`gmp-core` 与 `management` module 的 build、vet、gofmt、单测和 CI；MySQL 真实适配契约与并发唯一性；HTTP `/api/v1` 契约；Core 层依赖 / 状态架构门禁；文档一致性检查。
- 必补：纯值输入 / 输出场景覆盖分流稳定、受众互斥、活动决策、回执状态转换；MySQL 测试覆盖染色 first-write-wins、执行 / 回执并发回放、重启读取。
- 必查边界：`gmp-core` 无任何 Store / Feed / Log / Runner / Deliverer / EventPublisher 与可变跨调用运行态；`experimentation` 仍零依赖；无来源成员快照；外部事件只首次回执发出。
- 本次不覆盖：生产数据和渠道、Outbox / 崩溃恢复、生产 API 认证 / 部署、Issue #88 全部实体 CRUD、未知 Core Go 使用者。
- 评估器重点攻击：包装后仍留下隐藏状态；切片或导入聚合经局部别名 / helper 参数产生共享底层数组修改；算法 Registry 带来可变状态；纯转换遗漏 UID / key / placements；MySQL 唯一性不等于仅一份流程副作用；HTTP 适配误改 JSON 契约。
- 设计稿：`docs/superpowers/specs/2026-11-08-pure-core-runtime-design.md`。
