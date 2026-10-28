# 变更证据：实验人群包 core 最小闭环（#64）

## 背景

既有实验臂由调用方逐个拼装，活动直接挂人群配置；分流修订或停用时，旧活动缺少共同失效边界。#64 已确认实验人群包为受众侧一等公民，运行参数与加载组件逻辑分开管理。产品澄清四项见 [任务卡](../../ai-loop/task-cards/64-experiment-audience-bundle.md)，本轮未偏离。

## 关键实现与文件

- `gmp-core/experimentation/component.go`、`service.go`：按分流身份注册、启动、停用及修订组件；运行中只允许受校验的比例和白名单更新。染色已有记录先于白名单；白名单先于新随机分配。停用/修订推进代际，旧包不能复启。
- `gmp-core/audience/group.go`、`service.go`：单配置可呈现为单组；包构造时检查身份、来源、分流修订、份号和臂完整性，服务发布并按身份查询包/组。组在每次活动求值前核对发布状态及组件代际。
- `gmp-core/campaign/treatment.go`、`decisiontable.go`、`campaign.go`、`service.go`、`decision.go`、`runner.go`：投放项、决策表行、活动放量公开入口改挂组；新挂和旧引用均失败关闭。决策表命中行和活动放量组的分流份号也进入决策 Award 与执行记录；回执边投递前检查旧组可用性。
- `gmp-core/campaign/*_test.go`、`gmp-core/scenarios/`、`gmp-core/cmd/gmp-demo/receiptflow.go`：迁移仓内调用方；新增基于双十一加购人群的多活动复用、逐臂求值/执行、比例白名单更新及停用回归，并验证未注册/停用组件不能挂接、行组与投放项组不同及活动放量组归因、停用后的回执边不再投递。保留原有对照组、互斥、推送不占 Slot 等用例。
- `gmp-core/experimentation/component_test.go`、`gmp-core/audience/bundle_test.go`：运行态、染色优先级、构造拒绝和发布生命周期的直接回归。

## 验证结果

以下命令均在本机对本工作区执行，退出码均为 `0`：

| 工作目录 | 命令 | 实际结果 |
|---|---|---|
| `gmp-core` | `gofmt -l .` | 无输出，退出码 0 |
| `gmp-core` | `go build ./...` | 无输出，退出码 0 |
| `gmp-core` | `go vet ./...` | 无输出，退出码 0 |
| `gmp-core` | `go test ./...` | 全包通过；无测试包正常报告 `[no test files]`，退出码 0 |
| `gmp-core` | `go test -count=1 ./...` | 全包实际重跑通过，退出码 0 |
| `gmp-core` | `go test -race -count=1 ./experimentation ./audience ./campaign ./scenarios` | 四个业务包通过，退出码 0 |
| 仓根 | `python3 test/docs/consistency_check.py` | `docs consistency: 全部通过（10 项检查）`，退出码 0 |
| 仓根 | `git diff --check` | 无输出，退出码 0 |

本机命令结果在最后代码/文档修订后复跑；当前文档不声称 CI、生产或浏览器验证。

## 破坏性变更（等价 CHANGELOG）

**活动侧公开挂接入口从直接接收 `audience.Definition` 改为接收人群组；继续传入裸 Definition 的调用将无法编译或构造失败。** 涉及 `TreatmentSpec.Who`、`Row.When`、`CampaignSpec.Rollout` 及访问器。仓内 `campaign`、`scenarios`、演示装配与测试已迁移：普通配置用单组，发布实验臂按包份号取组。无兼容重载；当前未发布稳定版本，拟仓内一次升级。仓外调用方及无兼容期安排**待人工确认**，不能声称仓外无依赖。

## 风险处理、未覆盖项与回滚边界

- 真风险·阻断：旧引用停用后触达、展示及回执边可能继续执行；组求值与活动新挂守卫失败关闭，跨层用例覆盖新旧引用。染色/白名单顺序与逐臂实际份号有不同值的断言；`go test ./...` 同时执行 `experimentation` 零依赖检查。
- 真风险·放行后续跟进：包、组和运行态为进程内实现，未接生产持久化和多实例同步；AQ3 本期仅保留活动+臂归因字段，未交付指标双视图查询/UI。
- 真风险·仅观察：仓外 Go 调用方未知，合入前需人工确认迁移与兼容安排。
- 伪风险：受众层无需自行分桶，求值继续委托实验服务。
- 未覆盖：在途调用与停用并发的生产协调、真实来源/渠道及多实例联调、浏览器 UI。迟到回执仍会记录历史执行结果并发布事实事件，随后因旧臂失效拒绝回执边投递；该事件重试语义待后续明确。
- 回滚：本轮未改生产存储或部署；若在合入前撤销工作区 diff，可回到旧公开 Go 契约。若合入后回滚，需让所有仓内及仓外 Go 调用方与 core 同版回退，不得只回退 core；已染色记录不能回填或重跑清除。本期无生产数据迁移脚本。

## 评估器与人工放行

独立只读评估器第三轮结论为 `PASS`，无阻断问题；前两轮 `FAIL` 的产物侧反例已修复并固化为测试。评估器建议不新增长期抽象记忆，相关边界已有自动化回归。大任务最终人工审核、仓外调用方与兼容期确认、产品侧合入放行均由调用方处理；本轮无 commit、push、PR 或 merge。
