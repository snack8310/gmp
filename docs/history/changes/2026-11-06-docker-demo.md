# 变更证据：运营演示容器构建与编排

关联 issue #86。用户在本会话确认双容器方案；任务卡见 `docs/ai-loop/task-cards/86-docker-demo.md`。

## 背景

运营演示原本需要本机分别运行 Node 页面入口与 Go 管理 API。页面入口只监听回环地址，并把 API 请求代理到回环地址；直接拆成独立容器后无法连接。用户希望先获得可构建、可启动的演示镜像，以便考虑部署。

## 关键实现

- 一个多阶段 Dockerfile 分别产出管理 API 与前端目标。API 运行镜像仅含可执行文件；前端运行镜像包含页面入口及其资源，不包含测试和导出脚本。
- Compose 在内部网络运行 API，等 API 的活动列表健康检查通过后再启动前端；只将前端端口映射到宿主机回环地址。API 不发布宿主机端口。
- 前端入口新增监听主机与 API 主机配置，显式空值拒绝启动；未设置时沿用本机回环默认值。请求和异常继续输出不含请求体的结构化日志。
- README 补充构建、启动、停止和数据边界。容器化没有改变管理 API 契约、领域语义和浏览器存储格式。

## 关键文件

- `Dockerfile`、`.dockerignore`、`compose.yaml`：镜像构建、构建上下文和服务编排。
- `frontend/demos/operations-flow/serve.mjs`、`serve.test.mjs`：代理主机配置及本机兼容、空配置和清理失败路径回归。
- `README.md`、`frontend/demos/operations-flow/README.md`：运行与限制说明。

## 验证结果

以下结果来自本机真实执行；关联送审提交的逐项原始输出与 SHA 校验在评估输入包中提供，不把临时脚本称为 CI 固化。

| 命令或检查 | 结果 | 证据摘要 |
|---|---|---|
| `docker compose config -q`、`docker compose build --no-cache` | 退出码 0 | Compose 配置有效，前端和 API 镜像构建成功。 |
| `docker compose up -d --force-recreate --wait` | 退出码 0 | API 健康后前端启动。 |
| 宿主机 HTTP 与容器端口检查 | 通过 | 页面、关键资源及同源活动 API 返回 200；API 容器无宿主机端口映射，前端仅绑定回环地址。 |
| 停止与重启 API | 通过 | API 停止时代理返回 502 与 `api_unavailable`；API 重启后新建活动读取返回 404，证明编辑状态未持久化。 |
| `node --test frontend/demos/operations-flow/*.test.mjs`、`node --check frontend/demos/operations-flow/serve.mjs` | 退出码 0 | 前端现有回归与新增入口配置、子进程清理用例通过。 |
| `go build ./...`、`go vet ./...`、`go test ./...`、`gofmt -l .` | 退出码 0，格式检查输出为空 | 在 `management/` 和 `gmp-core/` 分别运行。 |
| `python3 test/docs/consistency_check.py`、`python3 test/docs/consistency_check_test.py`、`git diff --check` | 退出码 0 | 文档门禁、变异用例和差异检查通过。 |

## 风险与未覆盖项

- 真风险·阻断已处理：代理目标错误会让页面可开但活动 API 不可用；真实 Compose HTTP 验证穿过同源代理，API 停止时返回可识别的 502。
- 真风险·阻断已处理：演示入口无鉴权，公开端口会允许访问者改写进程内活动；Compose 默认仅绑定宿主机 `127.0.0.1`，外部访问须先配置 TLS 和访问控制。
- 真风险·仅观察：API 重启恢复构造种子，受众、实验与流程实验草稿只在当前浏览器；README 已明确。
- 未覆盖：远端部署、TLS、访问控制、生产持久化、多实例、真实数据和真实渠道；Docker 镜像没有推送到仓库或镜像服务。
- 外部读 / 写影响：本机 Docker 守护进程构建镜像并临时启动容器；HTTP 请求只读写构造演示的进程内状态。无生产数据库、缓存、消息队列或渠道写入。
- 回滚：撤销本分支的 Dockerfile、Compose、前端入口配置和文档即可恢复原本机运行方式；无数据迁移或回填。合并仍由人工放行。
