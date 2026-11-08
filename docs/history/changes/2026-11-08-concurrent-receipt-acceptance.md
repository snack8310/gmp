# 变更证据：并发相同回执只发布和沿边一次

关联 Issue #88、PR #89。用户批准修复独立评估器发现的并发回执竞态，并在评估器 `PASS` 后明确授权合并当前切片。

## 背景

`Runner.RecordReceipt` 先查询执行记录，再更新回执。两个并发请求可能都先读到无回执；即使 MySQL 最终只保存同一回执值，两条 Runner 路径仍可能各自发布事件并推进回执边。

## 关键实现

- 新增 `campaign.ReceiptAcceptor`，原子返回本次是否首次接受；内存日志使用互斥锁，MySQL 日志在事务内锁定执行记录并写入执行状态与回执。
- `Runner` 仅在首次接受时发布事件和推进回执边；相同重放无副作用，冲突回执仍被拒绝。
- `NewRunner` 对缺少原子接受能力的日志实现失败关闭。该 Go 运行时契约变化及适配器迁移方式记入 `CHANGELOG.md`；管理 HTTP API、数据库 schema 与存量数据格式未改变。
- 新增 Core 内存日志并发回归，以及两 Runner 连接真实 MySQL、竞争同一回执时只发布一次事件并只调用一次边目标的集成回归。

## 关键文件

- `gmp-core/campaign/execution.go`、`runner.go`：原子回执能力与首次接受后续行为。
- `management/internal/management/mysql_execution_log.go`：MySQL 事务和执行行锁。
- `gmp-core/campaign/flow_test.go`、`management/internal/management/mysql_execution_log_integration_test.go`：并发副作用回归。
- `CHANGELOG.md`、`docs/ai-loop/task-cards/88-core-mysql-business-layer.md`：破坏性运行时契约与任务状态。
- `docs/history/memory/receipt-edge-audience-partition.md`：记录持久化幂等与流程副作用幂等的区别。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `gmp-core`：`go test ./... -count=1` | 通过 | 含两个并发相同回执读写已被栅栏同步的 Runner 回归。 |
| `management`：`go test ./... -count=1` | 通过 | 在本机合成 MySQL 8.4 上运行；含两个独立 MySQL Store 与两个 Runner 的同回执并发用例。 |
| Core 与 MySQL Runner 并发用例：`go test -race` | 通过 | 两条并发路径均无竞态报告。 |
| 两模块：`go build ./...`、`go vet ./...`、格式检查 | 通过 | Core 与 management 分别执行；改动 Go 文件由 `gofmt` 格式化。 |
| `python3 test/docs/consistency_check.py`、`git diff --check` | 通过 | 文档一致性检查全部通过；差异无空白错误。 |
| GitHub `gates` | 通过 | run 105 对送审 HEAD `acfe1d6cd6361ead844403a7ae575426c5c49e2a` 返回 `completed/success`。 |
| 独立只读评估器 | `PASS` | 代码裁决审查 `963d88c3c4e52a8afde04226c43925453b54f004`；最终载体复核审查 HEAD `acfe1d6cd6361ead844403a7ae575426c5c49e2a`，无阻断问题、无缺失验证。 |

## 送审与合并授权

- 用户明确要求合并当前切片；最新送审载体评估为 `PASS`。据此将 PR #89 转为 Ready 并按 squash 规则合并；Issue #88 因完整验收范围仍未完成而保持开放。实际合并状态与提交 SHA 以 GitHub PR #89 元数据为准。

## 风险与未覆盖项

- 真风险（放行后续跟进）：Outbox 不在本期。回执事务提交后若事件发布或边处理失败，重复回执不会自动恢复未完成副作用；本期不承诺跨进程可靠投递或崩溃恢复。
- 真风险（用户已授权合并当前切片）：`NewRunner` 要求自定义执行日志实现 `ReceiptAcceptor`；旧自定义适配器若仅实现 `Log`，构造 Runner 将返回 `ErrReceiptAcceptanceUnavailable`。迁移说明已写入未发布变更记录。用户明确要求合并当前已通过评估的切片；Issue #88 保持开放，不代表其余架构目标或生产接入已验收。
- 本次不覆盖：Core Store / Feed / Log / Runner 完整迁出、生产用户池 schema 与权限、真实渠道幂等性、执行 / 回执保留周期及历史数据迁移。
- 外部读 / 写影响：仅本地构造 MySQL 8.4 中的合成记录；无生产数据库、真实用户名单或真实渠道交互。新增代码路径对 MySQL 执行表和回执表进行事务写入，并输出不含密钥或成员载荷的结构化操作日志。
