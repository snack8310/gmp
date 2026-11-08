# gmp
用户增长

## 项目结构

- `gmp-core/`：Go 业务模型、规则与回归场景。当前代码仍有内存 Store；目标是按 [Issue #88](https://github.com/snack8310/gmp/issues/88) 将存储移到外层业务应用。
- `management/`：独立 Go module，V1 活动与投放项管理 REST API（进程内状态）。
- `frontend/`：前端展示层，目前包含独立、零框架依赖的 [运营流程沙盘 Demo](frontend/demos/operations-flow/README.md)。
- `docs/`：领域规格、产品设计与协作证据。

## 运营流程立体展示

全部是构造数据，不接真实渠道，也不依赖 `gmp-core` 服务。活动编辑需要本机管理 API；分别启动：

```sh
cd management && go run ./cmd/campaign-api -addr 127.0.0.1:8082
node frontend/demos/operations-flow/serve.mjs 8081 8082
```

浏览器打开 `http://127.0.0.1:8081`。浏览器默认以页面同源地址访问活动 API，前端入口经 HTTP 转给管理 API；`serve.mjs` 换监听端口时无需改浏览器基址。单文件离线打开时默认无法管理活动。详见 [运行说明](frontend/demos/operations-flow/README.md)。

原命令行领域演示保持不变：在 `gmp-core` 下执行 `go run ./cmd/gmp-demo`。

## Docker 试运行

在仓库根目录执行：

```sh
docker compose build
docker compose up -d --wait
```

打开 `http://127.0.0.1:8081`。需要换宿主机端口时，在启动命令前设置 `GMP_DEMO_PORT`，例如 `GMP_DEMO_PORT=18086 docker compose up -d --wait`。停止演示用 `docker compose down`。Compose 只把页面端口绑定到宿主机回环地址；管理 API 仅供容器内的页面代理访问。

这套容器运行的是构造数据演示，没有鉴权、生产存储或真实触达。API 容器重启后活动编辑恢复构造种子；受众、实验及活动流程实验草稿仍只在当前浏览器，不会随容器部署到其他设备。若要供外部访问，先在页面端口前配置 TLS 和访问控制。详细边界见 [演示运行说明](frontend/demos/operations-flow/README.md)。

## 活动管理 API（本机演示）

在 `management/` 下运行 `go run ./cmd/campaign-api -addr 127.0.0.1:8081`。接口提供活动、投放项的增删改查，以及动作、人群引用目录和占用查询。数据由服务端内存持有，进程重启恢复构造种子；它不读取前端 Demo 的浏览器存储。请求、响应及限制见 [活动管理 REST API](docs/api/campaign-management-api.md)。
