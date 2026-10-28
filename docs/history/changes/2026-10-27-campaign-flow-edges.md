# 变更证据：活动流程与边

关联：issue #53

## 背景

事实 68–70 已在 main：活动内串行 / 按回执分支以活动流程的边为准，一个投放项仍一个动作。`gmp-core/campaign` 的投放项注释和执行路径仍把「投放项互不相识、下一步只能建在回流人群上」写成活动内的做法。没有可执行的边，这条裁定在运行时不存在。

## 关键实现

- **活动流程（`Flow`）**挂在活动上，不是投放项的字段。节点是投放项，各仍一个动作。
- **边（`Edge`）**连接两个投放项。种类必填：`receipt` 等待动作契约里的一个回执结果（成功 / 失败 / 契约另列的结果）；`parallel` 只表示两侧都是入口，不触发、不等待。漏填种类会在构造期失败，避免被当成并行。
- 动作可以声明回执结果。一旦声明，必须同时有成功和失败。边上的结果必须是源动作声明过的。回执上的结果不从「是否送达」推断。
- 回执边只把回执所属的那个人交给目标投放项，不枚举目标人群、不重查放量。非入口投放项不能再按人群批量跑。下一步的幂等键由上游执行键推导；同一回执再记一次，已落定的下一步不再投递。一条回执只走一跳。
- 没有活动流程、或有流程但回执边不连的多个投放项，仍各自是入口。
- `RecordReceipt` 增加活动参数，这样沿哪条边走由这次传入的活动流程决定。仓库内调用点已同步。
- 场景 ①⑤ 仍是两个活动靠回流时间窗相连。边上的等待时长未表态，没有改成边。
- 产品英文名（D2）未在领域文档关闭。本包使用 `Flow` / `Edge`，不把领域文档改成已定名。

## 关键文件

- [`gmp-core/campaign/flow.go`](../../../gmp-core/campaign/flow.go)：活动流程、边、构造期拒绝（环、未声明的结果、拉类目标、并行与回执冲突）
- [`gmp-core/campaign/action.go`](../../../gmp-core/campaign/action.go)：动作契约的回执结果
- [`gmp-core/campaign/runner.go`](../../../gmp-core/campaign/runner.go)：回执之后沿匹配的边投递一跳
- [`gmp-core/campaign/flow_test.go`](../../../gmp-core/campaign/flow_test.go)：串行、RCS 成功→邮件 / 失败→短信、契约其他结果、无边并行
- [`gmp-core/campaign/treatment.go`](../../../gmp-core/campaign/treatment.go)、[`gmp-core/campaign/event.go`](../../../gmp-core/campaign/event.go)、[`gmp-core/scenarios/twostep.go`](../../../gmp-core/scenarios/twostep.go)、[`gmp-core/cmd/gmp-demo/main.go`](../../../gmp-core/cmd/gmp-demo/main.go)：去掉「活动内多步只靠互不相识的投放项」的现行说法
- [`docs/ai-loop/task-cards/53-campaign-flow.md`](../../ai-loop/task-cards/53-campaign-flow.md)：入口检查

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `gofmt -l .`（`gmp-core`） | 退出码 0 | 无待格式化文件 |
| `go test ./...`（`gmp-core`） | 退出码 0 | `audience`、`campaign`、`experimentation`、`internal/arch`、`scenarios` 通过。分层检查在 `internal/arch` |
| `go vet ./...`（`gmp-core`） | 退出码 0 | 无诊断 |
| `go run ./cmd/gmp-demo`（`gmp-core`） | 退出码 0 | 演示进程正常结束 |
| `python3 test/docs/consistency_check.py` | 退出码 0 | 文档一致性门禁通过 |

## 风险与未覆盖项

- 真风险（放行后续跟进）：回执事件仍不去重，与引入边之前相同。重复记同一条回执会再发一条到达事件；下一步投递本身会因键相同而停住。
- 真风险（仅观察）：执行记录多了回执结果字段。当前只有内存日志，没有已落库的数据要迁。
- 本次不覆盖项：边上的「等待 N 天」；同步受理自动当成契约成功；活动 / 投放项 / 活动流程的运行态状态机（D1）；领域文档里的产品英文名（D2）；场景 ①⑤ 不改写成边。失败的边步骤在同一回执再次记下时会用同一键再试，这条恢复路径没有单独的场景用例，已落定（非失败）的下一步不会再投递则有用例。
- 人工确认：活动流程与边的语义已由事实 68–70 确认并在 main。issue #53 要求按该语义实现。边的种类无零值、沿边不重查目标人群、同步受理不合成成功，是按「不得臆造默认值」作的实现裁定，见任务卡。
- 需要后续动作的事项：若要边上的时间条件或流程运行态，另开任务；不要在本结构上把回流时间窗塞进边。
