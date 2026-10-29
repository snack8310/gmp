# 变更证据：V1 活动管理 REST API

关联 issue #69；分支 `feat/69-campaign-management-api`，基于 `origin/main` 的 `636aa45`。本机 Codex CLI 落地后由执行器复核、提交并开 PR；**尚未 merge**，合入前仍须产品/用户放行。

## 背景

前端活动演示的浏览器状态不能作为活动管理的共同写入边界。本轮建立独立管理服务，使活动、投放项、动作与人群引用校验由服务端承担。实施前用户已确认服务端构造目录、服务端分配 ID、占用查询不强制浏览器侧禁删、默认不改 `gmp-core` 公开契约；见[任务卡](../../ai-loop/task-cards/69-campaign-management-api.md)。

## 关键实现

- `management/` 是独立 Go module。启动时加载构造活动、动作、人群引用与展示位目录；进程内存储用互斥锁包住整份校验和替换。服务端分配活动、投放项 ID；改名保持 ID，删除后当前进程不重生种子，重启重新构造初始状态。
- HTTP 资源路由提供活动、投放项增删改查，目录与当前活动占用查询；JSON 请求拒绝未定义字段与错误媒体类型。普通组以 `groupId`，包臂以 `bundleId` 加 `groupId` 判身份。失败写入不保存半份活动。
- 展示动作必须使用已知展示位；推送不得占位。同活动同位且无决策表时，仅同包不同臂视为可证互斥。删除被活动流程边引用的投放项返回冲突；活动流程只读保留，不提供边编辑。
- CI 为新 module 增加格式、构建、静态检查与测试门禁。HTTP 测试覆盖目录身份、可用性、增删改查、整份写入、重启时重新构造、失败原子性、展示位、互斥、边引用、占用变化及请求边界反例。

## 关键文件

- `management/go.mod`、`cmd/campaign-api/main.go`：独立 module 与启动入口。
- `management/internal/management/model.go`、`seeds.go`、`store.go`：管理侧数据结构、构造目录、校验与内存状态。
- `management/internal/httpapi/http.go`、`http_test.go`：JSON 路由、结构化日志和 HTTP 正反例。
- `docs/api/campaign-management-api.md`、`README.md`：最终接口契约、启动方式与 V1 边界。
- `.github/workflows/gates.yml`：新 module 的 CI 检查。

## 验证结果

以下结果来自本机真实执行。Go 命令在 `management/` 下，其他命令在仓根；CI 尚未执行。

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `gofmt -l .` | 退出码 0，输出为空 | Go 文件无待格式化项。 |
| `go build ./...` | 初次退出码 1 | 本机默认 Go 构建缓存目录无写权限，不是源码编译错误。 |
| `GOCACHE=/private/tmp/gmp-69-go-build-cache go build ./...` | 退出码 0 | 换用可写临时缓存后全包构建通过。 |
| `go vet ./...` | 退出码 0 | 无诊断。 |
| `go test ./...` | 退出码 0 | HTTP 测试通过。 |
| `go test -count=1 ./...` | 退出码 0 | 不依赖测试缓存重新执行通过。 |
| `go test -race -count=1 ./...` | 退出码 0 | 当前测试路径无竞态报告。 |
| `python3 test/docs/consistency_check.py` | 退出码 0 | 文档一致性门禁通过。 |
| `git diff --check` | 退出码 0 | 已跟踪 diff 无空白错误；新文件另用 `git diff --no-index --check /dev/null <file>` 逐个检查，均无空白诊断。 |
| `go run ./cmd/campaign-api -addr 127.0.0.1:18069` + 独立 HTTP 客户端（Codex 沙箱内） | 未通过环境验证 | Codex 沙箱拒绝监听，`bind: operation not permitted`。 |
| 执行器在沙箱外：`go build -o … ./cmd/campaign-api` 后监听 `127.0.0.1:18081`，独立 `curl` 客户端 | 退出码 0 / HTTP 断言通过 | `Content-Type: application/jsonp` → 415；合法 JSON 创建活动 201；创建投放项（`actionId`+普通组）201；`application/json; charset=utf-8` 201。 |

## 风险与未覆盖项

- 真风险（阻断，代码侧已处理）：绕过前端提交错误动作、人群、展示位或同位冲突会写坏活动；服务端每次写入校验整份候选活动，在锁内一次替换，HTTP 反例证明拒绝后状态不变。独立评估首轮指出 `application/jsonp` 被误收与普通组空 `bundleId=` 被误收，已用精确媒体类型解析和参数存在性检查修复并补回归。
- 真风险（仅观察）：进程重启丢失编辑结果；受众目录仅取服务端构造种子，不接浏览器 localStorage。`audience-reference-usages` 随服务端活动变化，但不能阻止浏览器侧受众删除。#68 已编辑受众进入服务端目录尚未解决。
- 未覆盖：生产持久化、多实例、鉴权、真实渠道、前端编辑器接线、活动流程边编辑、浏览器矩阵和生产部署均未做。Codex 沙箱内独立监听曾失败；执行器在沙箱外已用真实进程与 `curl` 补证媒体类型与 CRUD 主路径，但不能代替评估器最终 `PASS`。
- 外部读 / 写影响：服务仅对 HTTP 客户端读写当前进程内状态；不读写生产数据库、缓存、消息队列、渠道或浏览器存储。GitHub issue 页面本机无法连接，实施依据为仓内任务卡和用户本轮规格。代码日志为结构化 JSON，不含请求体、凭据或个人信息。
- 回滚：撤回本分支 `management/`、API 文档、README 和 CI 变更即可；没有生产数据迁移、回填或外部配置变更。合入前仍需人工最终放行。
