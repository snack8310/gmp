# Issue #90：纯化 gmp-core 并迁出状态编排

## 背景

`gmp-core` 原先包含运行态服务、存储接口和有状态编排；这使纯业务模型与 MySQL、回执处理等应用职责混在一起。本次按已确认的破坏性迁移方案，将运行态所有权移到 `management`，Core 只保留显式值、确定性规则和状态转换。

## 关键实现

- 移除 Core 中的状态服务、Runner、Store / Feed / Log 与内存实现 API，将流程编排和 MySQL 执行、回执、染色存储迁至 `management`。
- Core 场景使用输入值计算结果；保留 `/api/v1` 契约，并在 `CHANGELOG.md` 记录 Go API 破坏性迁移。
- `gmp-core/internal/arch` 增加随 Core 测试运行的边界门禁，覆盖运行态所有权、依赖与时钟访问；补充导入结构体的切片字段浅拷贝后交给修改 helper 的窄反例。
- 收窄了切片别名门禁实现：未保留评估过程中引入的通用参数追踪分析器。

## 关键文件

- `gmp-core/internal/arch/arch.go`：Core 运行态所有权与切片共享边界门禁。
- `gmp-core/internal/arch/arch_test.go`：门禁正反例，包括导入切片字段被浅拷贝后交给 helper 的反例。
- `gmp-core/campaign/pure.go`、`gmp-core/audience/facts.go`、`gmp-core/experimentation/`、`gmp-core/scenarios/`：纯业务转换和场景迁移。
- `management/internal/application/runtime.go`、`management/internal/application/runtime_mysql_integration_test.go`：外层应用编排与 MySQL 执行、回执验证。
- `docs/ai-loop/task-cards/90-pure-core-runtime.md`、`docs/domain/business-scenarios.md`、`CHANGELOG.md`：任务风险、场景语义和破坏性变更记录。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `GOTOOLCHAIN=go1.24.0 go test -C gmp-core ./... -count=1` | 通过 | Core 全量测试通过；定向切片浅拷贝反例在修复前失败、修复后通过。 |
| `GOTOOLCHAIN=go1.24.0 go test -C management ./... -count=1` | 通过 | management 全量测试通过。 |
| MySQL 专项集成 | 通过 | 本地 MySQL 8.4 的临时 schema 验证回执和染色并发行为；临时 schema 已删除，项目容器已停止。 |
| 两个 Go module 的 `go build ./...`、`go vet ./...` | 通过 | `gmp-core` 与 `management` 均通过构建和静态检查。 |
| `gofmt`、`git diff --check`、`python3 test/docs/consistency_check.py` | 通过 | 格式、差异空白和文档一致性检查通过。 |
| `python3 test/docs/consistency_check_test.py` | 通过 | 文档门禁变异测试通过；夹具纳入 `management/`，以覆盖文档链接指向的迁移后运行时文件。 |
| 独立只读评估器 | `PASS` | 预提交实现快照评估通过；当时 Draft PR 与送审 SHA 对应的 CI 尚未创建。 |

## 风险与未覆盖项

- 真风险（放行后续跟进）：Core 静态门禁维护 AST 类型与别名识别，未来修改时应保留浅拷贝共享切片反例，并继续验证合法只读用法。
- 本次不覆盖：生产用户池、真实渠道、Outbox 和崩溃恢复、生产部署及 Issue #88 全部 CRUD。
- 需要后续动作：无 Outbox 的恢复限制由后续设计处理；仓外 Go importer 按 CHANGELOG 中的迁移要求升级。
