# 变更证据：活动流程的跨层回归包

关联：issue #55

## 背景

活动内串行、按回执分支、无边并行已经在 `gmp-core/campaign` 里成立。`scenarios` 与 `gmp-demo` 里没有同一套装配：`TwoStep` 是两个活动靠回流（含时间窗），`ArmCampaign` 是一个活动挂住同一次实验的各个臂。缺了跨层回归，活动内下一步可以退回「再建一份人群」，而包内单测仍然全绿。

## 关键实现

- 新增 `Stack.ReceiptFlow()`。demo 与回归用例都调用它。三条路径各是一个活动、各有自己的 runner；事件丢弃，不经回流来源把下一步圈出来。
- 串行：入口 `recall-entry` 成功回执指向 `recall-next`。下一步不是入口。它的人群是另一份非空分区（returning），与入口（new）不相交。
- 分支：同一出口，成功到邮件、失败到短信。两边回执都标了送达，边只看契约结果。
- 并行：一条 `parallel` 边，两侧都是入口，人群一为 new、一为加购，大小不同且有交集。跑一侧、以及给它记成功回执，都不会启动另一侧。
- 一个投放项一个动作：装配里每个投放项对上用例写明的一个动作；`Treatment` / `TreatmentSpec` 上出现第二个 `Call`、`[]Call` 或 `Call` 数组时用例失败；不填 `Call` 时构造失败。
- 边上的等待时长和渠道「未读」写在 `Missing`，不近似成回执结果。`TwoStep` 与 `ArmCampaign` 的装配不改。
- 位子沿用既有 `BannerSlot`、`SMSSlot`、`CouponSlot`。同一活动内每个投放项用其中不同的一个，避免属性分区过不了抢位证明。这是沿用，不裁定 #45。

## 关键文件

- [`gmp-core/scenarios/receiptflow.go`](../../../gmp-core/scenarios/receiptflow.go)：三条路径的共享装配
- [`gmp-core/scenarios/receiptflow_test.go`](../../../gmp-core/scenarios/receiptflow_test.go)：串行、非入口拒绝、回执分支、无边并行、一投放项一动作
- [`gmp-core/cmd/gmp-demo/receiptflow.go`](../../../gmp-core/cmd/gmp-demo/receiptflow.go)：demo 打印节，不断言输出快照
- [`gmp-core/cmd/gmp-demo/main.go`](../../../gmp-core/cmd/gmp-demo/main.go)：在两级触达之后调用该节。实验臂那一节的函数体未改
- [`gmp-core/scenarios/doc.go`](../../../gmp-core/scenarios/doc.go)：说明 `ArmCampaign`、`ReceiptFlow`、`TwoStep` 是三种装配
- [`docs/ai-loop/task-cards/55-flow-edge-scenarios.md`](../../ai-loop/task-cards/55-flow-edge-scenarios.md)：入口检查

## 验证结果

命令在最终工作区执行。`go` 命令的工作目录是 `gmp-core`，文档门禁在仓库根。`gofmt -l .` 在仓库根执行。

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `gofmt -l .` | 退出码 0 | 无待格式化文件 |
| `go build ./...` | 退出码 0 | 模块内包完成构建 |
| `go vet ./...` | 退出码 0 | 无诊断 |
| `go test ./...` | 退出码 0 | `audience`、`campaign`、`experimentation`、`internal/arch`、`scenarios` 通过 |
| `go run ./cmd/gmp-demo` | 退出码 0 | 进程结束。回执边一节打印出：非入口批量跑被拒绝；成功与失败人数不同且下一步人数等于成功人数、不等于下一步自己的人群人数；分支交叉为 0；并行两侧人数不同，先跑一侧时另一侧为 0 |
| `python3 test/docs/consistency_check.py` | 退出码 0 | 文档一致性门禁通过 |

判别力抽查（改完即还原，最终 diff 不含这些改动）：

- 把串行下一步的人群改成与入口相同的 `tier=new`：`TestReceiptSerialReachesOnlyThePeopleOnTheReceipt` 失败，下一步不再指向用例写明的 returning 人群。
- 把并行边改成成功回执边：`TestParallelEntriesRunTheirOwnAudiences` 失败，边的种类与用例写明的 `parallel` 不一致。
- 临时放宽 `campaign/runner.go` 的边匹配，使成功边接受任何回执结果，随后还原该文件：`TestReceiptSerialReachesOnlyThePeopleOnTheReceipt` 与 `TestReceiptBranchSendsEachOutcomeDownItsOwnEdge` 失败，失败的人也走进了成功边。

`git diff` 未改 `ArmCampaign`、`ExperimentArms`、`twostep.go` 与 `campaign` 包。

## 风险与未覆盖项

- 真风险（仅观察）：位子取值沿用既有三个槽位，不裁定 #45。
- 本次不覆盖项：边上的等待时长；渠道「未读」；多臂与边的组合；实验指标；真实渠道。前两项记在装配的 `Missing`，仍由 `TwoStep` 表达。
- 需要后续动作的事项：若要多臂与边的组合，或要边上的时间条件，另开 issue。不要把回流时间窗塞进本装配。
