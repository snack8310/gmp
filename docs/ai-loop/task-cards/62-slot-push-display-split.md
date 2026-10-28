# 任务入口检查：推送投放项与展示位拆分

关联 issue：#62（承接 #45）｜分支：`feat/62-slot-push-display-split`

## 任务分类

- 类型：新需求开发。
- 等级：大任务。修改核心领域构造契约，跨 `campaign`、`scenarios` 和领域文档；issue 标记 `level/large`、`risk/high`、`risk/breaking`。
- 产品确认：2026-10-28（Asia/Shanghai）用户已裁定 Slot 只表示展示位，推送不占展示位；当前用户明确要求按 #62 实施。最终合入仍待人工放行。

## 问题本质

- 表面请求：清除推送投放项的虚构 Slot，并调整构造期校验。
- 真实问题：现有必填 Slot 让推送借用展示位，导致错误配置被接受，还把展示位互斥和注册要求施加给推送。
- 硬约束：已知展示动作必须带展示位；推送必须不带 Slot；未知的未来 Pull 动作不能因为方向被强制归为展示；展示位的抢位、优先级、决策表行为不变。
- 已否决的更本质解法：直接把 `Slot` 改为可选而不区分动作，会放过已知展示动作缺位；按 `DirectionPull` 推断展示，则把未来所有 Pull 动作锁死。因此在 `ActionSpec` 增加默认 `false` 的 `RequiresSlot`，传入 `Action` 和 `Call`。当前 `show-image` 显式开启。`NewTreatment` 据此检查 Slot；推送的非空 Slot 报独立错误。未标记的 Pull 可不带 Slot，且带 Slot 报错。
- 成功标准：issue #62 验收标准均有代码、场景或文档证据；本机工程集命令通过。

## 范围与调用方

- 范围内：`gmp-core/campaign/action.go`、`treatment.go`、`campaign.go` 与相关测试；`gmp-core/scenarios/stack.go`、`setup.go`、`twostep.go`、`receiptflow.go` 与测试；领域事实 7、23 和不变量 9；本任务卡及变更证据。决策表只增加“目标投放项必须占该展示位”的构造校验，有效决策表的匹配顺序与兜底行为不变。
- 已核对调用方：`campaign` 内 `harness_test.go`、`guard_test.go`、`execution_test.go`、`flow_test.go`、`scenario_test.go` 的构造辅助与断言；`scenarios` 内 `scenario_test.go`、`receiptflow_test.go`；`gmp-core/cmd/gmp-demo` 通过 `scenarios.Stack` 间接调用，无直接 `NewTreatment` 或虚构 Slot 装配。`campaign/service.go` 的 `Add` 通过 `Campaign.slots()` 取待注册展示位；只需让 `slots()` 忽略空值。`campaign/runner.go` 的推送执行无需读取 Slot。
- 非目标：推送互斥、频控、编辑器、生产渠道；展示位跨活动抢位与决策表语义；frontend demo 业务语义。
- 外部系统读 / 写影响：本轮仅修改内存领域模型及本地场景，生产数据库、缓存、消息队列、渠道均无读写变更；`gh issue view 62` 只读 issue 正文。

## 高风险项

| 项目 | 判定 | 依据 |
|---|---|---|
| 通用 1 核心领域语义 | 是 | `Slot` 改为仅表示展示位，改变投放项构造规则。 |
| 通用 2 被依赖的接口契约 | 是 | `ActionSpec` 新增标记；已有推送携 Slot 的 `NewTreatment` 调用改为失败。 |
| 通用 3 幂等 / 重试 / 并发 / 顺序 | 否 | 不改 Runner、幂等键或执行顺序。 |
| 通用 4 持久化结构 / 迁移 | 否 | 不改持久化数据结构或含义；本仓库场景仅用内存。 |
| 通用 5 权限 / 认证 / 审计 | 否 | 无鉴权或审计改动。 |
| 通用 6 多实现一致 | 否 | 不改多套执行态实现的行为。 |
| 通用 7 跨模块或不可重跑自愈 | 是 | `campaign` 构造契约和 `scenarios` 装配需同时迁移；错误推送不可通过重跑撤回。 |
| 项目清单 2 组件契约 | 是 | `ActionSpec` 是动作注册契约，需保证标记传到 `Call` 和 `Treatment`。 |
| 项目清单 14、15 决策表与抢位 | 否 | 仅限制同活动展示位守卫的适用对象；决策表顺序、兜底、互斥及跨活动优先级不改。 |
| 其余项目清单项 | 否 | 不改事件、回执边、执行记录、人群、分桶、权益幂等、渠道、存储或依赖分层。 |

## 破坏性变更与兼容策略

**已存在的「推送投放项携带 Slot」调用将由构造成功变为构造失败。** 受影响的仓库内调用方列于上节，并在本分支同步改为无 Slot；`scenarios.SMSSlot` / `CouponSlot` 原为导出常量，本轮删除会使引用它们的源码编译失败。

`scenarios.ArmTreatment.Slot` 保留字段，但推送描述中的值改为空；依赖该字段非空的调用方需迁移为按 `Action` 与人群组识别推送。

原先能构造的跨展示位决策表路由（包括把无 Slot 推送指向展示位）现在由 `ErrRowTreatmentWrongSlot` 拒绝；调用方需让表的目标投放项真正占用该展示位，或移除对推送的展示位决策表。

- 受影响的仓库外调用方范围：仓库内搜索只能证明仓内用法；Go module 可被仓外引用，不能断言没有依赖。仓库未见发布 tag，也未见独立 `CHANGELOG`；二者均不足以证明仓外无人使用。
- 迁移方式：推送投放项移除 `Slot` 字段；已知展示动作在 `ActionSpec` 显式填 `RequiresSlot: true` 并保留真实展示位。`ErrUnexpectedSlotID` 与 `ErrMissingSlotID` 供调用方辨别。
- 版本化、兼容期与废弃计划：本分支不发布版本，也不设置静默兼容期，因为兼容旧推送 Slot 会继续接受已被裁定为错误的配置。对外发布的版本安排、仓外调用方核实及是否接受无兼容期，均**待人工确认**；未经确认不进入合入或发布。本次 `docs/history/changes/2026-10-28-slot-push-display-split.md` 作为等价对外变更记录，记录调用迁移与破坏性摘要。

## 人工确认

- 实施前产品语义：**已确认**，来源为 #45 用户裁定、#62 正文及当前用户指令；当前用户已明确授权在指定分支实施并留下 diff。
- 仓外调用方与发布兼容策略：**待确认**，需人工核查仓外依赖并批准版本化、无兼容期方案。issue #62 另要求合入前产品侧再次询问用户是否放行，**待确认**；本轮不 commit、push、开 PR 或 merge。

## 历史规则命中

| 来源 | 规则 / 位置 | 判定与应用 |
|---|---|---|
| `docs/history/memory/mechanization-claims.md` | 规则 1、8、9（行 5、17–18） | 命中。构造约束只称代码校验和本轮本机测试；场景期望独立写出，逐个消耗的既有用例复核下限。其余规则不涉及新增结构性检查或固化归属表。 |
| `docs/history/memory/source-enumeration-fidelity.md` | 规则（行 5–11） | 命中。任务卡与变更证据逐项列影响面，不写冻结的测试数量或条目数量。 |
| `docs/history/memory/receipt-edge-audience-partition.md` | 规则（行 5–9） | 命中。保留串行 / 分支下一步与回执人不相交的非空人群及原断言。 |
| `docs/history/memory/review-package-consistency.md` | 规则（行 5–12） | 部分命中。任务卡与变更证据确认状态一致；无 PR、送审 SHA、CI 引用或外部写入载体，本轮其余规则不适用。 |
| `docs/history/memory/shell-script-portability.md` | 规则 1–5 | 不命中：不改 shell 脚本或钩子。 |
| `docs/history/changes/2026-09-24-one-campaign-many-arms.md` | 风险与未覆盖项 | 命中：推送位子在批量路径无用途，#45 留下的风险由本轮构造校验处理。 |
| `docs/history/changes/2026-10-28-flow-edge-scenarios.md` | 关键实现、风险与未覆盖项 | 命中：原场景沿用虚构 Slot 是待 #45 裁定的债，本轮清除；回执边原语义不改。 |
| `docs/history/changes/2026-10-28-campaign-orchestrator.md` | 活动流程裁定 | 命中：只迁移投放项 Slot，不改活动流程的边。 |
| `docs/domain/campaign-user-stories.md` | 事实 7、23、66 | 命中：同位互斥限于展示；展示位仍对应展示动作。 |
| `docs/domain/interaction-invariants.md` | 不变量 9 | 命中：建活动时仅拦展示位争用。 |

- 扫描方式：`rg --files docs/history/memory docs/history/changes` 后读取全部记忆，并优先读取近期活动流程、场景与多动作变更证据；按 `rg` 检索 `Slot` / `#45` 追溯同领域历史。未逐篇阅读与本轮无关的受众命名、shell 等变更流水。

## 最小验证集与约束落地

- 必跑：`gofmt -l .`、`go build ./...`、`go vet ./...`、`go test ./...`（`gmp-core`）；`python3 test/docs/consistency_check.py`、`git diff --check`（仓库根）。
- 必补：推送空 Slot 成功、任意非空 Slot 失败；已标记展示缺 Slot 失败、带 BannerSlot 成功；未标记 Pull 不被强制占展示位；两个无 Slot 推送可入同一活动且 `Service.Add` 不要求注册空 Slot；决策表的行与兜底不能把无 Slot 推送路由为展示结果；展示同位守卫及既有决策表成功样例。
- 必查：banner 抢位与决策表；同臂短信加券；无边并行入口；回执边后续步。检索并清除场景 / demo 对虚构 Slot 的引用，保留真实 banner Slot。
- 不覆盖：仓外源码迁移与发布验证、真实渠道、生产数据、推送互斥 / 频控 / 编辑器；不做浏览器桌面走查。
- 约束落地：`ActionSpec.RequiresSlot` 由注册和调用链保存，`NewTreatment` 在构造边界拒绝错误 Slot；单测与场景回归守住两侧。无需新增散文规则。
- 回滚：撤回本分支代码与文档 diff；本轮不改数据，无迁移或生产回滚步骤。

## 风险点与评估器重点

### 真风险

- **阻断**：当 `RequiresSlot` 未传入 `Call` 时，展示动作缺位会被错放行；评估器应检查传递链及缺位错误测试。
- **阻断**：当空 Slot 被作为普通键进入同位守卫或 `Campaign.slots()` 时，两个推送会被误判冲突，或 `Service.Add` 要求注册空 Slot；用同活动双推送直接验证。
- **阻断**：当现有展示动作未显式标记时，banner 缺位会被错放行；评估器逐个核对装配与测试构造器。
- **阻断**：当决策表只按目标 ID 核对归属而不核对目标展示位时，无 Slot 推送可被路由为 banner 展示结果；行与兜底均须在 `NewCampaign` 拒绝。
- **仅观察**：去掉展示位守卫不能解释成允许同渠道重复推送；本 issue 不裁定该策略。

### 伪风险

- 短信与券不再分占虚构 Slot 会失去同臂双动作：两者仍是独立投放项，`Runner` 按各自 `Call` 执行，场景回归核对。
- 推送无 Slot 会改变跨活动 banner 抢位：`Service.Decide` 只按非空展示位检索，banner 场景保持原测试。

### 未讨论项

- 仓外调用方及发布版本如何迁移？交人工在合入前核查和确认；评估器检查本轮是否误称无需兼容。
- 非 Pull 展示动作将来如何标记？本轮只给已知 `show-image` 打标；评估器攻击未标记 Pull 的默认行为，未来动作接入另行显式标记。

评估器还须核对上述历史规则、真实命令退出结果、破坏性记录、无需注册空 Slot 的反例，并判断是否需要新增抽象记忆。
