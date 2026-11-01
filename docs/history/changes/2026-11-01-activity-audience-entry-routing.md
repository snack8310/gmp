# 变更证据：活动唯一受众与入口路由

## 背景

活动原先可挂多个入口人群，投放项也能各自带人群，导致实验分臂与活动内回执路径混在一起。按确认后的模型，活动只选择一个普通人群组或一个实验人群包；画布负责把入口组/实验臂路由到起始投放项，后续只沿动作回执边推进。

## 关键实现

- Core 将活动受众收敛为一个封闭选择，并要求活动持有流程；投放项不再带独立人群。
- 普通组或每个实验臂通过显式流程入口连接起始投放项；对照臂显式为空路由；回执 UID 沿边进入后续投放项。
- 管理 API `/api/v1` 原地切换为单受众合同，允许保存逐步配置的草稿；前端仅在入口完整、投放项唯一且可达、流程无环时允许本地演练。
- 保留已有 SVG 3D 画布和 CSS；活动、受众、实验列表共享既有资产，演练只使用构造样本。
- 更新历史记忆：移除“目标投放项需绑另一份非空且不相交人群”的旧前提，改为由类型边界及 UID、execution key、placements 回归验证沿边关联。

## 关键文件

- `gmp-core/audience/selection.go`：普通组或实验包的单一活动受众选择。
- `gmp-core/campaign/{campaign,flow,runner,service,treatment}.go`：活动、入口、投放项与回执执行语义。
- `management/internal/{management,httpapi}/`：单受众与活动流程的 V1 数据合同及保存校验。
- `frontend/demos/operations-flow/activity-flow-canvas.mjs`：复用 3D renderer，保存入口草稿并门控演练。
- `frontend/demos/operations-flow/activity-flow-canvas.test.mjs`、`gmp-core/campaign/{flow_test,guard_test}.go`、`gmp-core/scenarios/receiptflow_test.go`：不完整实验臂禁止演练、决策表候选不可跨臂，并覆盖回执 UID、execution key 与 placements。
- `docs/domain/`、`docs/api/campaign-management-api.md`、`CHANGELOG.md`：模型、API 与 V1 破坏性变更记录。
- `docs/history/memory/receipt-edge-audience-partition.md`：修订回执边长期记忆以匹配当前类型边界。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `GOTOOLCHAIN=go1.24.0 go test -count=1 ./...`（`gmp-core/`） | 退出码 0 | audience、campaign、experimentation、internal/arch、scenarios 均通过；含跨实验臂决策候选回归。 |
| `GOTOOLCHAIN=go1.24.0 go vet ./...`（`gmp-core/`） | 退出码 0 | 无诊断。 |
| `GOTOOLCHAIN=go1.24.0 go test -count=1 ./...`、`GOTOOLCHAIN=go1.24.0 go vet ./...`（`management/`） | 退出码 0 | HTTP API 与管理模型测试通过；无 vet 诊断。 |
| `GOTOOLCHAIN=go1.24.0 go build -o /tmp/gmp-entry-routing-api-final ./cmd/campaign-api`（`management/`） | 退出码 0 | 管理 API 构建成功。 |
| `node --test --test-reporter=dot *.test.mjs`（`frontend/demos/operations-flow/`） | 退出码 0 | 前端测试通过，含完整实验臂与缺失起始路由门禁。 |
| `node campaign-http-check.mjs` 及两份脚本 `node --check` | 退出码 0 | V1 HTTP 回归覆盖单活动受众、实验臂入口、冲突拒绝与活动更新。 |
| CUA 实际页面操作 | 通过 | 空置 B 臂可保存为草稿；演练按钮显示“补全流程后演练”且禁用；恢复 A/B/对照路由后按钮可用，3D 流程可见。 |
| `python3 test/docs/consistency_check.py` | 退出码 0 | 10 项文档一致性检查通过。 |
| `gofmt -l .`（`gmp-core/` 与 `management/`）、`git diff --check`、CSS diff 检查 | 通过 | 两个 Go 模块均无待格式化文件；diff 无空白错误；没有 CSS 文件变更。 |
| 独立只读评估器 | PASS | 未发现阻断问题；CDP 全量浏览器脚本未运行，CUA 实测覆盖了关键画布和不完整草稿门禁。 |

## 风险与未覆盖项

- 真风险（放行后续跟进）：完整 CDP `browser-check.mjs` 因本机没有 CDP 端口未运行；本次通过 CUA 覆盖画布渲染、部分实验臂草稿保存、演练禁用及完整路由恢复。
- 非阻断观察：`gmp-core/campaign/flow_test.go` 的少量旧注释仍提到目标投放项匹配空人群；Treatment 已无此字段，后续可单独清理注释。现有 UID、execution key 与 placements 断言通过。
- 本次不覆盖项：真实渠道、生产持久化、生产授权、跨实例管理、真实分桶压力与非 Chromium 浏览器。
- 对外破坏性变更：`/api/v1` 请求与响应结构原地更新；没有迁移计划或 V2 并行兼容，已由用户确认并在 `CHANGELOG.md` 记录。
- 回滚：回退本分支提交；不涉及外部系统写入或持久化迁移。演示 API 运行时的测试草稿已恢复为完整 A/B/对照入口。
