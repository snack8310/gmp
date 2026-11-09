# gmp-core

`gmp-core` 只承载业务值、构造校验、确定性规则、决策和状态转换。它不读写数据库，不查询人群，不保存执行状态，也不投递动作或发布事件。需要存储的状态以显式值传入 Core，Core 返回下一状态或外部效果意图；应用层负责读取、提交、并发、幂等、重试、调度与副作用。

Issue #90 完成了 Core 运行态边界迁移。旧 `Service`、`Runner`、Store / Feed / Log 接口及 Memory* 实现已删除；仓库调用方迁到 `management/internal/application` 和 MySQL 适配器。`gmp-core` Go API 属于破坏性变更，迁移入口见根目录 [`CHANGELOG.md`](../CHANGELOG.md)。管理 HTTP `/api/v1` 契约保持不变。

领域语义的规格真源是 [`docs/domain/`](../docs/domain/)，不是本目录。发现规格有洞时走澄清，不在 Core 里添加未确认的业务事实。

## 依赖分层

下层不得依赖上层，依赖方向由 [`internal/arch`](internal/arch) 检查：

| 包 | 可以依赖 | 职责 |
|---|---|---|
| `experimentation` | 无 | 确定性分桶、显式染色事实、组件状态值转换 |
| `audience` | `experimentation` | 人群定义、规则校验、显式记录匹配与份号放置 |
| `campaign` | `audience` | 活动配置、抢位决策、投递意图、执行 / 回执纯转换 |
| `scenarios` | 以上全部 | 五个已建立跨层业务场景的纯值回归 |

`experimentation` 的零依赖与四个 Core 包不得声明运行时所有权类型、存储接口或外部副作用依赖，均由 `go test ./...` 中的 AST 门禁检查。空目录、空源码、禁止类型和禁止依赖都有门禁测试。

## 纯值 API

- `experimentation.Calculate` / `ResolveAssignment` 接收显式算法、assignment key、definition、可选 component state 和已有染色，返回份号及待持久化染色值。数据库唯一键和首次写胜由外层处理。
- `audience.Definition.Matches`、`Placements` 与 `Selection.Match` 只读取显式记录和 `AssignmentFacts`。`Source` 是来源描述值，不是查询端口；成员列表由外层适配器提供。
- `campaign.Decide`、`PlanEntry`、`CompleteDelivery` 与 `ApplyReceipt` 返回决策、投递意图、执行值或回执转换。Core 不执行这些意图；应用层需先原子接受回执，再执行事件与后续投递。
- `ComponentState`、`Bundle.Publish` 等 API 以值输入和返回新值表达状态转换。Core 不在多个调用之间保存运行态。

## 业务场景覆盖

场景测试位于 `scenarios/`，只构造业务值，不使用内存数据库或伪造的运行时 Store。下表与根 [`AGENTS.md`](../AGENTS.md) 保持一致：

| 场景 | 状态 | 当前回归覆盖 |
|---|---|---|
| ① 新客首单转化 | **部分建立** | 注册满 24 小时、券成功投递事件回流与三天时间窗；“未使用”判定仍未建立 |
| ② 双十一预热分组实验 | **分组、对照组与触达已建立** | 四臂分割、显式对照入口与其他三臂起始路由；指标未建立 |
| ③ APP 首页 banner 个性化 | **已建立** | 新客 / 老客决策表、活动优先级抢位与失败留痕 |
| ④ 新玩法灰度放量 | **放量单调性已建立** | 5% → 20% → 100% 扩大既有稳定分桶集合；执行部分未建立 |
| ⑤ 跨境召回 · 两级触达 | **已建立** | 四小时窗口、未读条件、投递 / 回执 key 关联、UID / placements 沿回执边传递 |

没有在表中扩大未确认场景。新客“未使用”仍是外部券系统持有的事实，是否由同一受众定义跨来源读取仍待业务确认；它不由本次改动推定。

## 管理应用与本地演示

MySQL、人群源读取、染色持久化、执行 / 回执记录及 `Deliverer` / `EventPublisher` 由独立 `management` module 的应用层与适配器实现。外部数据库影响面如下：

- `gmp_demo_user_pool` 只读；适配器按显式列映射做 keyset 分页，不缓存或复制名单。
- 平台执行、回执和染色数据写入 `gmp_management_*` 表；染色使用新增 migration 003，不回填旧数据。
- Demo 投递和事件只写结构化日志，不连接真实渠道。

纯值演示不需要数据库：

```sh
cd gmp-core
go run ./cmd/gmp-demo
```

MySQL 运行演示需要本机 MySQL、已导入的构造用户池表，以及从安全配置来源提供的四项 `MYSQL_*` 环境变量。先将 [`internal/management/demo_user_pool.sql`](../management/internal/management/demo_user_pool.sql) 应用到本地开发数据库，再运行：

```sh
cd management
# 从安全配置来源注入 MYSQL_DSN、MYSQL_MAX_OPEN_CONNS、MYSQL_MAX_IDLE_CONNS、MYSQL_CONN_MAX_LIFETIME_SEC。
go run ./cmd/gmp-demo
```

演示会运行纯 Core 规则，通过同一个 MySQL 连接池读取构造人群，并将执行与回执写入平台表。它不自动创建或填充人群表，不接生产用户池，也不发真实触达。MySQL 集成验证由 `GMP_TEST_MYSQL_DSN` 指向构造测试库。

## 本地验证

```sh
(cd gmp-core && go test ./... && go vet ./... && go run ./cmd/gmp-demo)
(cd management && go test ./... && go vet ./...)
```
