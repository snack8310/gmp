# gmp
用户增长

## 项目结构

- `gmp-core/`：Go 业务值、规则、校验、决策、状态转换与纯值回归场景；不持有内存或持久化存储，也不执行外部副作用。说明见 [`gmp-core/README.md`](gmp-core/README.md)。
- `management/`：独立 Go module，V1 活动管理 REST API 与 Core 应用运行时。管理配置、人群读取、染色、执行 / 回执 MySQL 持久化及渠道 / 事件端口均在 Core 外；MySQL Demo 使用构造数据，不接真实渠道。
- `frontend/`：前端展示层，目前包含独立、零框架依赖的 [运营流程沙盘 Demo](frontend/demos/operations-flow/README.md)。
- `docs/`：领域规格、产品设计与协作证据。

## 运营流程立体展示

活动配置写入由调用者配置的平台 MySQL，前端受众 / 实验规则和模拟名单仍是构造数据；不接真实用户池、渠道或 `gmp-core` 服务。活动编辑需要本机管理 API；分别启动：

```sh
cd management
# 从安全配置来源注入 MySQL 配置，不要写入仓库或日志。
export MYSQL_DSN
export MYSQL_MAX_OPEN_CONNS
export MYSQL_MAX_IDLE_CONNS
export MYSQL_CONN_MAX_LIFETIME_SEC
go run ./cmd/campaign-api -addr 127.0.0.1:8082
```

另一个终端启动同源入口：

```sh
node frontend/demos/operations-flow/serve.mjs 8081 8082
```

浏览器打开 `http://127.0.0.1:8081`。浏览器默认以页面同源地址访问活动 API，前端入口经 HTTP 转给管理 API；`serve.mjs` 换监听端口时无需改浏览器基址。单文件离线打开时默认无法管理活动。详见 [运行说明](frontend/demos/operations-flow/README.md)。

Core 纯值演示不连接数据库：

```sh
cd gmp-core
go run ./cmd/gmp-demo
```

## MySQL 业务运行 Demo

此演示通过 management 应用运行 Core 流程，需使用本机 MySQL 和构造用户池。先将 [`management/internal/management/demo_user_pool.sql`](management/internal/management/demo_user_pool.sql) 应用到本地开发数据库；然后从安全配置来源注入 `MYSQL_DSN`、`MYSQL_MAX_OPEN_CONNS`、`MYSQL_MAX_IDLE_CONNS` 和 `MYSQL_CONN_MAX_LIFETIME_SEC`，运行：

```sh
cd management
go run ./cmd/gmp-demo
```

Demo 只读 `gmp_demo_user_pool`，并将执行与回执写入平台自有表。投递 / 事件使用本地日志适配器，不连接真实渠道；不会自动创建或导入名单数据，也不要将生产 DSN 用于此演示。

## Docker 试运行

在仓库根目录执行：

```sh
docker compose build
docker compose up -d --wait
```

打开 `http://127.0.0.1:8081`。需要换宿主机端口时，在启动命令前设置 `GMP_DEMO_PORT`，例如 `GMP_DEMO_PORT=18086 docker compose up -d --wait`。停止演示用 `docker compose down`。Compose 只把页面端口绑定到宿主机回环地址；管理 API 仅供容器内的页面代理访问。

Compose 启动前必须从安全配置来源注入 `MYSQL_DSN`、`MYSQL_MAX_OPEN_CONNS`、`MYSQL_MAX_IDLE_CONNS` 和 `MYSQL_CONN_MAX_LIFETIME_SEC`；API 会将活动配置写入 DSN 指向的平台自有 MySQL。Compose 不包含数据库服务或凭据。服务没有鉴权、真实人群来源或真实触达；受众、实验及活动流程实验草稿仍只在当前浏览器，不会随容器部署到其他设备。新 schema 不自动导入演示种子。若要供外部访问，先在页面端口前配置 TLS 和访问控制。详细边界见 [演示运行说明](frontend/demos/operations-flow/README.md)。

## 活动管理 API（本机演示）

在 `management/` 下从安全配置来源注入四项必填 `MYSQL_*` 变量后，运行 `go run ./cmd/campaign-api -addr 127.0.0.1:8081`。接口提供活动、投放项的增删改查，以及动作、人群引用目录和占用查询。活动配置及普通人群引用写入平台 MySQL；API 不读取前端的圈选规则或名单样本。外层还提供同一连接池的执行 / 回执日志和 demo 用户池只读分页适配器，但未接入 API 的运行流程。当前实现面向演示与架构验证，不代表生产规模就绪。请求、响应及限制见 [活动管理 REST API](docs/api/campaign-management-api.md)。
