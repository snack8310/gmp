# 变更证据：推送投放项与展示位拆分

关联：issue #62，承接 #45｜日期：2026-10-28（Asia/Shanghai）

## 背景

#45 的用户裁定已确认：`Slot` 只表示展示位，推送不强制也不允许挂展示位。旧构造器要求所有投放项填写 Slot，短信、发券、活动流程推送因此借用 `BannerSlot`、`SMSSlot`、`CouponSlot`，导致展示位同位互斥与注册要求误用于推送。

## 关键实现

- `ActionSpec.RequiresSlot` 显式标记已知展示动作；现有 `show-image` 注册点开启该标记。标记经 `Action` 和 `Call` 传到 `NewTreatment`，未标记 Pull 不因方向被永久当成展示。推送动作不得声明需要展示位。
- `NewTreatment` 对标记展示动作缺 Slot 返回 `ErrMissingSlotID`，对推送或其他非展示动作填入任意 Slot 返回 `ErrUnexpectedSlotID`。推送无 Slot 可构造。
- `NewCampaign` 的同位守卫只聚合带展示位的投放项；`Campaign.slots()` 忽略空 Slot，使仅含推送的活动通过 `Service.Add` 时无需注册虚构展示位。决策表目标必须占用该表展示位：行与兜底都在构造期校验，防止无 Slot 推送绕过投放项校验后被路由为 banner 展示结果。有效展示位的跨活动抢位、决策表顺序及兜底行为未改。
- 场景装配中的短信、发券、WhatsApp、RCS、邮件、通知等推送均不带 Slot；banner 继续使用 `BannerSlot`。移除虚构的 `SMSSlot` / `CouponSlot` 常量；实验同臂的短信与券仍是独立投放项，回执边与并行入口仍按原业务路径执行。demo 的实验臂说明改为推送不占展示位。
- 领域事实 7、23 和不变量 9 只把同位互斥施加给展示位；活动侧新增 #45 用户裁定记录。

## 关键文件

- `gmp-core/campaign/action.go`、`treatment.go`：动作展示标记的构造链及 Slot 错误。
- `gmp-core/campaign/campaign.go`：空 Slot 不参与同位守卫与待注册位集合；决策表行 / 兜底目标与展示位一致性校验。
- `gmp-core/campaign/slot_split_test.go`：推送、已标记展示动作、未标记 Pull、纯推送活动、决策表错配的构造回归。
- `gmp-core/campaign/harness_test.go`、`execution_test.go`、`flow_test.go`：展示测试动作显式标记；推送测试辅助不再填 Slot。
- `gmp-core/scenarios/stack.go`、`setup.go`、`twostep.go`、`receiptflow.go`：共享场景装配清除虚构推送 Slot；`ArmTreatment.Slot` 保留但推送值为空。
- `gmp-core/scenarios/scenario_test.go`、`receiptflow_test.go`、`twostep_test.go`：保留 banner 抢位与决策表、同臂短信加券、并行入口、回执边后续步的回归，显式核对推送不占展示位。
- `gmp-core/cmd/gmp-demo/main.go`：实验臂解释与新的 Slot 语义对齐。
- `docs/domain/campaign-user-stories.md`、`interaction-invariants.md`：事实、不变量及裁定记录。
- `docs/ai-loop/task-cards/62-slot-push-display-split.md`：入口检查、调用方与破坏性兼容安排。

## 验证结果

除明确标注外，Go 命令工作目录为 `gmp-core`，其余命令在仓库根执行。以下结果来自本机真实执行；`go test -count=1 ./...` 用于排除缓存结果。

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `gofmt -l .` | 退出码 0 | 输出为空，无待格式化 Go 文件 |
| `go build ./...` | 退出码 0 | 全部 Go 包构建通过 |
| `go vet ./...` | 退出码 0 | 无诊断 |
| `go test ./...` | 退出码 0 | `audience`、`campaign`、`experimentation`、`internal/arch`、`scenarios` 通过 |
| `go test -count=1 ./...` | 退出码 0 | 同一组包重新执行通过，未依赖测试缓存 |
| `go run ./cmd/gmp-demo` | 退出码 0 | banner、实验臂、两步回流、活动流程与演练均完成本地演示 |
| `python3 test/docs/consistency_check.py` | 退出码 0 | 文档一致性门禁通过 |
| `git diff --check` | 退出码 0 | 当前工作区差异（含新增文件）无空白错误 |

判别力：新测试 `TestPushTreatmentsHaveNoDisplaySlot` 与 `TestUnmarkedPullActionDoesNotRequireDisplaySlot` 在修改前因缺 Slot 而失败；评估器发现的决策表行与兜底路由反例在修复前均失败，`NewCampaign` 增加目标展示位校验后通过。原有 banner 抢位与决策表、同臂短信加券、并行入口、回执边后续步回归随 `go test ./...` 通过。

## 破坏性变更（等价对外变更记录）

**已存在的「推送投放项携带 Slot」调用将由构造成功变为构造失败。** 调用方需删除推送 `TreatmentSpec.Slot`；已知展示动作须在 `ActionSpec` 明确设置 `RequiresSlot: true` 并保留真实展示位。`scenarios.SMSSlot`、`CouponSlot` 已删除，引用它们的源码需迁移；`ArmTreatment.Slot` 字段保留，但推送描述中的值改为空。跨展示位的决策表路由（包括把无 Slot 推送作为展示目标）现在由 `ErrRowTreatmentWrongSlot` 拒绝，表目标须占用同一展示位。

本仓库没有独立 `CHANGELOG` 文件，本节作为本次对外变更记录。仓内调用方已同步；Go module 可能被仓外引用，仓库搜索和未发现发布 tag 均不能证明仓外没有依赖。兼容策略是在本分支直接拒绝旧错误配置，不保留静默兼容期。对外版本化、仓外调用方范围、无兼容期及废弃安排**待人工确认**；当前不合入、不发布。#62 所要求的合入前产品侧再次询问用户放行亦**待确认**。

## 风险与未覆盖项

- 真风险（阻断，已处理）：当决策表目标只按 ID 校验而不校验展示位时，无 Slot 推送可成为 banner 展示结果；独立评估器首轮发现，现由 `NewCampaign` 构造校验和行 / 兜底反例测试约束。
- 真风险（仅观察）：移除推送的展示位守卫，不构成对同渠道推送重复触达的产品承诺；本 issue 不设推送互斥。
- 仓外调用方与发布兼容安排：**待人工确认**，不能仅凭本仓库测试宣布对外兼容。
- 本次不覆盖：推送互斥、频控、编辑器、真实渠道与生产数据；不做浏览器走查。外部系统读 / 写影响：无生产数据库、缓存、消息队列或渠道变更；读取 issue #62 正文用于核对规格。
- 回滚：撤回本分支代码与文档 diff。无数据迁移、回填或生产配置变更。

## 评估器

独立只读评估器首轮裁决 `FAIL`，指出决策表可把无 Slot 推送路由成展示结果；修复并重跑验证后复审裁决 `PASS`。评估器认为该反例已由构造校验与测试承接，不建议新增抽象记忆。最终合入放行仍由人工决定。
