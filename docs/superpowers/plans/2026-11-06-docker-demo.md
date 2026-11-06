# 运营演示容器化实施计划

> **供实施者执行：**按以下步骤顺序完成，使用当前会话的主 Agent 实施，并交独立只读评估器审查。

**目标：**让运营演示可以通过 Docker 构建并由 Compose 启动前端与管理 API。

**架构：**一个 Dockerfile 提供独立镜像目标。Compose 用内部服务名连接前端代理和 API，只映射前端回环端口；现有本机启动保持默认回环地址。

**技术栈：**Go 1.24、Node、Docker、Compose。

**设计：**`docs/superpowers/specs/2026-11-06-docker-demo-design.md`。

## 全局约束与审查重点

- 保留现有 `serve.mjs 8081 8082` 和管理 API 启动方式；不改 HTTP 契约、领域语义或浏览器数据格式。
- 容器 API 不发布宿主机端口；前端默认仅绑定宿主机 `127.0.0.1`。
- 重点攻击 API 未就绪、错误上游主机、空主机配置、端口误暴露、重启后的数据误认。

## 文件职责

- `frontend/demos/operations-flow/serve.mjs`：可配置监听和代理上游主机，保留本机默认行为。
- `Dockerfile`：构建 Go 管理镜像目标和 Node 前端镜像目标。
- `compose.yaml`：独立启动服务、建立内部连接并限制端口映射。
- `.dockerignore`：排除无关构建上下文。
- `README.md`、`frontend/demos/operations-flow/README.md`：部署命令和数据边界。
- `frontend/demos/operations-flow/serve.test.mjs`：代理配置与失败路径回归。

## 实施步骤

### Task 1: 前端代理配置

- [ ] 编写前端入口测试：显式上游主机可代理 API，空主机配置拒绝启动，默认本机命令仍连回环地址。
- [ ] 运行测试确认新增配置尚未实现。
- [ ] 在 `serve.mjs` 读取 `GMP_DEMO_LISTEN_HOST` 与 `GMP_DEMO_API_HOST`，默认 `127.0.0.1`，空值报错；代理及监听使用相应主机。
- [ ] 运行入口测试与现有前端回归。

### Task 2: 容器构建和编排

- [ ] 写 Dockerfile 的 Go 编译目标、API 运行目标与 Node 前端目标；运行镜像不复制测试或无关源码。
- [ ] 写 Compose 的内部 `api` 服务、前端代理环境、回环端口映射及 API 就绪检查；写 `.dockerignore`。
- [ ] 运行 `docker compose config -q` 与 `docker compose build`；启动后验证页面、同源 API、API 无宿主机映射及重启边界。

### Task 3: 文档与最终验证

- [ ] 更新根目录和演示 README，列出启动、停止、访问及数据限制。
- [ ] 运行管理 API 构建与测试、前端回归、文档门禁和差异检查。
- [ ] 整理真实命令结果供独立只读评估器审查，依据裁决修复并记录变更证据。
