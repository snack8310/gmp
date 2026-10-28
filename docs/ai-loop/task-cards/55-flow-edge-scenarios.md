# 任务入口检查：活动流程的跨层回归包

关联 issue：#55 ｜ 分支：`feat/55-flow-edge-scenarios`

## 任务分类

- 类型：新需求开发
- 等级：**大任务**
- 判断依据：issue #55 标明 `level/large`；装配横跨 `scenarios` 与 `gmp-demo`，要同时守住串行、回执分支、无边并行、回执带人、一投放项一动作。不改 `campaign` 里已落地的边语义，故不把本轮算作活动流程语义的首次引入。

## 问题本质

- 用户表面请求：为活动流程 Flow+Edge 补一份本地跨层回归包，并在 `gmp-demo` 里能看到。
- 要解决的真实问题：串行、按回执分支、无边并行在 `campaign/flow_test.go` 里已经成立，但运营会配的那个形状没有经 `scenarios.Stack` 跑通过，也没有演示。缺了这一层，活动内下一步可以悄悄退回「再建一份人群」，而包内单测仍然全绿。
- 硬约束：
  - H1：demo 与回归用例调用同一个 `scenarios.Stack` 装配。
  - H2：不改 `ArmCampaign` / `ExperimentArms` 的含义与断言，不把边塞进多臂装配。
  - H3：演示节同时有跨层用例；演示不是保障。
  - H4：一个投放项一个动作。
  - H5：回执边上的下一个人是回执上的那个人。
  - H6：不引入新的领域概念；边表达不了的等待与「未读」写进装配的 `Missing`。
  - H7：不改 `docs/domain/`。实现若与事实 68–70 冲突，停下来澄清，不改文档凑绿。
- 已考虑并否决的更本质解法：把 `TwoStep` 改成边。时间窗和「未读」仍然不是回执结果，改掉它会让「边上没有时间条件」失去对照。把边挂进 `ArmCampaign` 同样否决：那是另一个形状，混在一起以后拆不开。
- 成功标准：三条路径都由 `Stack.ReceiptFlow()` 跑出来；非入口批量跑得到 `ErrNotAnEntry`；下一步的人等于命中边的回执人，且与下一步自己的人群不是同一批；`ArmCampaign` 的既有路径无语义改动。

## 范围

- 范围内：`gmp-core/scenarios` 的 `ReceiptFlow` 装配与用例；`gmp-demo` 的一节打印；本任务卡；评估通过后的变更证据。
- 非目标：改 `ArmCampaign`；多臂与边的组合；实验指标；frontend / 运营沙盘；时间窗边；真实存储、渠道、HTTP、配置文件；改 `docs/domain/`。
- 影响面：`gmp-core/scenarios` 新增装配；`gmp-core/cmd/gmp-demo` 增加一节；`gmp-core/scenarios/doc.go` 的包注释说明三个装配各是什么。`campaign`、`audience`、`experimentation` 的类型与执行路径不改。
- 外部系统读写影响：无。仍走内存投递与内存日志。

## 高风险项

| 高风险项 | 是 / 否 | 依据 |
|---|---|---|
| 通用 1 核心领域模型语义 | 否 | 不改活动流程、边、回执或执行的类型与分支。调用已有的 `Flow` / `Edge` / `RecordReceipt` |
| 通用 2 被依赖的接口契约 | 否 | 不改已有函数签名。新增的是 `Stack` 上的装配，既有调用方不经过它 |
| 通用 3 幂等 / 重试 / 顺序 | 否 | 不改幂等键、重试或沿边一跳的实现 |
| 通用 4 持久化结构 | 否 | 不改执行记录的字段，无迁移 |
| 通用 5 权限 / 认证 / 审计 | 否 | 无鉴权面 |
| 通用 6 多实现必须一致 | 否 | 不新增第二种执行实现 |
| 通用 7 跨模块且错误不可自愈 | 否 | 不改变发出去的路径。错配会在本轮用例里红，而不是改线上语义 |
| 清单 1 活动流程 / 边的语义 | 否 | 语义已由事实 68–70 与 `campaign` 实现落地。本轮是调用方回归，不是首次把边引进模型。若把「scenarios 里第一次调用」也算首次引入，每加一条回归都会命中本条，卡点不再能区分 |
| 清单 2–17 其余 | 否 | 不改组件契约、人群圈选、分桶、执行三段关联、权益幂等、资格、渠道协议、执行态实现、迁移、授权、决策表、抢位、依赖分层、时间来源 |

- 是否为对外破坏性变更：否
- 其他高风险项：无

## 人工确认

- 是否需要实施前人工确认：否（高风险项均为否，不触发额外卡点）
- 需要确认的问题：无
- 已确认事项与来源：issue #55 正文是本轮规格；本次用户指令要求按该 issue 实施并开 PR。事实 68–70 已在 main，本轮不改领域文档。最终合并仍由人工执行。

## 历史规则命中

### 规则级命中清单

| 来源文件 | 规则 / 章节 / 行号 | 是否命中 | 应用方式 / 不适用原因 |
|---|---|---|---|
| `docs/history/memory/mechanization-claims.md` | 规则 1 | 命中 | 行为由 `go test` 覆盖；在进入 CI 前不称为已固化 |
| 同上 | 规则 2–7 | 不命中 | 不新立「已固化」声明，不改检查脚本，不写固化归属表 |
| 同上 | 规则 8 | 命中 | 回执人、成功边、失败边、两个并行入口的人群彼此不能是同一批常量；期望写成用例里的集合关系 |
| 同上 | 规则 9 | 不命中 | 本轮用例不逐个消耗被测集合的成员 |
| `docs/history/memory/source-enumeration-fidelity.md` | 禁止在送审载体书写计数 | 命中 | 任务卡、变更证据、PR 正文、commit message 不写断言数、用例数、条目数 |
| `docs/history/memory/review-package-consistency.md` | 载体一致、送审 SHA、人工确认三态 | 命中 | 评估前取 PR head SHA 核对；确认状态只写已确认 / 未确认 / 待确认 |
| `docs/history/memory/shell-script-portability.md` | 规则 1–5 | 不命中 | 不改 shell |
| `docs/history/changes/2026-09-15-scenarios-and-demo.md` | 「装配根是共享的，不是各写一套」 | 命中 | `ReceiptFlow` 放在 `scenarios` 包，demo 调用它 |
| `docs/history/changes/2026-09-24-one-campaign-many-arms.md` | `ArmCampaign` 的形状与「期望不从被测对象反读」 | 命中 | 不改该装配；本轮期望在用例里另写 |
| `docs/history/changes/2026-10-27-campaign-flow-edges.md` | 边的种类、回执带人、非入口拒绝批量跑 | 命中 | 只调用，不重写 `runner` / `flow` |
| `docs/history/changes/2026-10-28-campaign-flow-first-class.md` | 活动流程是活动上的对象 | 命中 | 边挂在活动的 `Flow` 上，不挂在投放项字段上 |
| `docs/history/changes/2026-09-15-events-and-backflow.md` | 两步靠回流 | 部分命中 | `TwoStep` 保持；`ReceiptFlow` 的 runner 丢弃事件，不靠回流把下一步圈出来 |
| `AGENTS.md` 全局约束 | 不得臆造默认值 | 命中 | 不发明边上的时间条件；位子沿用既有 `BannerSlot` / `SMSSlot` / `CouponSlot` |
| `AGENTS.md` 约束落地优先级 | 类型先于文档 | 命中 | 一个投放项一个动作沿用 `Treatment` 上的一个 `Call`；本轮用用例钉住，不另写散文禁令 |
| `AGENTS.md` 业务回归用例 | 场景 ①⑤ 仍走回流 | 命中 | 不把 `TwoStep` 改写成边 |
| `docs/domain/campaign-user-stories.md` | 事实 68、69、70 | 命中 | 只读。串行与分支走边，并行是没有回执边指向的入口。不改该文件 |

### 历史扫描记录

- `docs/history/memory/` 扫描方式：逐份阅读 `mechanization-claims.md`、`source-enumeration-fidelity.md`、`review-package-consistency.md`、`shell-script-portability.md`。
- 近期 `docs/history/changes/` 扫描方式：同领域优先看活动流程、scenarios 装配、回流。上表是筛选结果。受众命名、实验人群包、设计概览 HTML、文档门禁与本轮边的调用方回归无关。
- 相关规格：`docs/domain/campaign-user-stories.md` 事实 68–70（只读）；issue #55 正文。
- 未读取来源及原因：无。

## 最小验证集

- 必跑命令：`gofmt -l .`、`go build ./...`、`go vet ./...`、`go test ./...`、`go run ./cmd/gmp-demo`（均在 `gmp-core`）、`python3 test/docs/consistency_check.py`（仓库根）。
- 必补测试：
  - `TestReceiptSerialReachesOnlyThePeopleOnTheReceipt`
  - `TestRunningTheReceiptTargetFromTheScenarioIsRefused`
  - `TestReceiptBranchSendsEachOutcomeDownItsOwnEdge`
  - `TestParallelEntriesRunTheirOwnAudiences`
  - `TestReceiptFlowTreatmentsEachCarryOneAction`
- 必查边界：回执尚未到达时下一步为零；成功与失败两边都非空，且「是否送达」不能区分它们（两边回执都标了送达）；下一步人群与回执人不是同一批；跑一个并行入口、以及给它记一条成功回执，都不会启动另一个入口。
- 可接受的替代证据：无。
- 本次不覆盖项：边上的等待时长；「未读」；多臂与边组合；实验指标；真实渠道。`Missing` 写明前两项。位子取值沿用既有 scenarios，不裁定 #45。

## 风险点

### 真风险

1. **阻断**：当下一步仍按自己的人群枚举时，人群若恰好与回执人相同，用例会全绿。装配把下一步指向另一份非空人群，用例要求执行 UID 等于回执人、并且与那份人群不相交。
2. **阻断**：当非入口仍能 `Run` 时，边和批量各发一次。用例要求 `ErrNotAnEntry`，且该动作的投递次数为零。
3. **阻断**：当 `Run` 在没有回执时就走出成功边，失败的人也会出现在下一步。用例先看回执前的投递，再把成功和失败分成两批。

### 伪风险

1. `TwoStep` 会因为新装配而坏掉。那两个活动没有活动流程，本轮不改 `twostep.go`。全量 `go test` 仍跑它们。
2. `ArmCampaign` 会被顺手改掉。装配在新文件，demo 的实验臂函数不改；只在 `run` 里多调用一节。
3. 属性分区不能证明互斥，活动会建不出来。同一活动里每个投放项用不同的既有位子，抢位检查不会被触发，也没有放宽。

### 未讨论项

1. 推送位子的语义仍是 #45。本轮澄清问题：这三个投放项该落在哪类位子上？处理：沿用 `BannerSlot` / `SMSSlot` / `CouponSlot`，变更证据写「沿用，不裁定」。评估器攻击点：是否发明了新的位子种类或新的抢位规则。
2. 同步受理算不算契约成功——#53 已裁定不算。评估器攻击点：`Run` 返回后、记回执之前，成功边是否已经投递。

## 约束落地方式

- 本次是否新增约束：否。钉住的是已有约束在跨层装配上的可见性。
- 若新增：一个投放项一个动作已是 `Treatment` / `TreatmentSpec` 上的一个 `Call`。用例在反射上看见第二个 `Call`、`[]Call` 或 `Call` 数组即失败，并在装配上逐个投放项核对动作。
- 若不能：等待时长不是本轮要固化的约束，保持未表态，写在 `Missing`。

## 评估器重点

- 需要评估器重点攻击的反例：
  1. 下一步人群被改成与回执人相同，用例仍绿。
  2. 用「送达 / 未送达」代替契约结果走边。
  3. 没有回执时成功边已经发出。
  4. 非入口批量跑只在 `campaign` 包内会红。
  5. 并行边被当成回执边，记一条成功回执后另一侧启动。
  6. `ArmCampaign` 或 `TwoStep` 被改了语义。
  7. `Missing` 被拿掉，等待被近似成某个回执结果。
  8. 领域文档被改来迁就实现。
- 需要评估器检查的历史规则：上表，含判定不适用的几条。
- 需要评估器核对的验证证据：上列命令的真实退出结果，且与送审 SHA 对应。
- 需要评估器判断是否新增抽象记忆：候选是「活动内下一步的人来自回执，scenarios 层要用一份与回执人不同的非空人群挡住巧合重建」。由评估器裁定，不由主 Agent 写入 `docs/history/memory/`。
