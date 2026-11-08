# gmp
用户增长

## 项目结构

- `gmp-core/`：Go 业务模型、规则与回归场景。当前代码仍有内存 Store；目标是按 [Issue #88](https://github.com/snack8310/gmp/issues/88) 将存储移到外层业务应用。
- `management/`：独立 Go module，V1 活动与投放项管理 REST API；活动配置、人群引用及执行 / 回执日志由平台自有 MySQL 持久化。合成用户池 SQL 与同连接池只读适配器已提供，但尚未接入 API 求值；Core 领域转换、实验定义与染色存储仍未接入。
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

原命令行领域演示保持不变：在 `gmp-core` 下执行 `go run ./cmd/gmp-demo`。

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
