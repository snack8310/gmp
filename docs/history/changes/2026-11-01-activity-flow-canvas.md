# 活动详情恢复 3D 流程画布

## 背景

活动导航调整后，活动详情只显示投放项和回执边列表，旧版 3D SVG 沙盘没有入口。用户要求保留活动为工作容器、受众与实验作为共享资产，并在活动内部先选择/创建受众，再装配投放项和回执流程；不得恢复独立「组合」菜单或重画画布。

## 关键实现

- 将 `41acbb9` 中历史 SVG 沙盘绘制与交互抽到活动详情：投影、节点绘制、视角切换、拖动旋转、路径演练均复用既有实现，并绑定当前活动数据。
- 为活动增加 `audienceRefs` 入口引用，支持在画布中选择/移除共享入口；起始投放项与入口引用必须一致。
- 普通共享人群保存或改名时，将稳定引用 ID 与名称同步到本机管理 API；圈选规则和模拟样本仍留在浏览器。投放项沿用已有编辑表单，回执边可在画布内新增、编辑并删除；编辑保留边 ID，同一画布的保存请求用 in-flight 锁禁止重复覆盖；服务端根据提交前的边/入口快照拒绝旧标签页覆盖新状态。
- 管理 API 校验入口、引用可用性、边端点、动作回执结果、重复路由与循环；失败写入不替换已保存状态。
- 本地演练仅使用构造样本与预设回执，不发起网络请求或真实渠道触达。
- 画布样式放在独立 CSS；核心 `frontend/demos/operations-flow/style.css` 未修改。

## 关键文件

- `frontend/demos/operations-flow/activity-flow-canvas.mjs`：图构造、历史 SVG 绘制/交互、本地演练、入口及回执边增删改。
- `frontend/demos/operations-flow/activity-flow-canvas.css`：活动画布局部样式。
- `frontend/demos/operations-flow/audience-ui.mjs`、`app.mjs`、`campaign-controller.mjs`：共享人群 ID/名称同步与活动工作区入口。
- `management/internal/management/model.go`、`store.go`、`management/internal/httpapi/http.go`：活动入口模型、引用注册与校验 API。
- `frontend/demos/operations-flow/browser-check.mjs` 与相关测试：活动绑定、受众创建/选择、演练和窄屏回归。
- `docs/api/campaign-management-api.md`、`frontend/demos/operations-flow/README.md`：本机 API 与产品操作边界说明。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `node --test --test-reporter=dot frontend/demos/operations-flow/*.test.mjs` | 通过 | 前端活动、受众、画布、API 客户端和原有回归通过。 |
| `go test ./...`（`management/`） | 通过 | 管理 HTTP API、活动入口引用及流程边校验通过。 |
| `go build -o /tmp/gmp-campaign-api-flow ./cmd/campaign-api`（`management/`） | 通过 | 管理 API 可构建。 |
| `python3 test/docs/consistency_check.py` | 通过 | 文档一致性检查全部通过（10 项）。 |
| `git diff --check` | 通过 | 无空白错误。 |
| `node frontend/demos/operations-flow/browser-check.mjs 8281 9243 /tmp/gmp-flow-evidence-final` | 通过 | 经静态服务代理本机 Go 管理 API，验证活动深链、空活动、普通受众新建与目录同步、放入入口后新增投放项且默认引用该受众、投放项在用时禁止移除入口，受众入口与回执边双重提交各只产生一次 PUT、过期标签页提交受众或流程边均以 409 拒绝且保留新状态；回执边新增/编辑起点终点和条件/删除并刷新后保留，模拟写失败后锁恢复，删除投放项后可移除入口、播放/暂停/重置、演练期间无网络请求和 390px 窄屏。宽屏画布截图（1440×1200）生成于 `/tmp/gmp-flow-evidence-final/activity-flow-f5e6b2ca-715c-40a7-9987-fed196ba8a7e.png`。 |
| 本地 8281 页面手动走查 | 通过 | 活动详情展示共享入口、投放项、回执分支和旧版风格 3D 画布；拖动旋转、节点选择可用。 |
| 独立只读评估器 | PASS | 无阻断；非阻断观察为「已走回执边」按总边数/播放进度估算、Safari/Firefox/真机触屏未覆盖；不建议新增抽象记忆。 |

## 风险与未覆盖项

- 真风险（放行后续跟进）：管理 API 是进程内演示状态，重启恢复构造种子；浏览器重载时会重新同步本地人群 ID 与名称。人群规则和模拟样本不复制到 API。
- 非阻断观察：画布「已走回执边」统计按总边数与播放进度估算，不是逐个构造样本的实际过边数；分支负载不同时可能与可见样本路径不同，不代表业务执行口径。
- 本次不覆盖项：真实渠道、生产发送、持久化/多实例、Safari/Firefox/真机触屏；复杂并行入口组合仅以当前构造活动和浏览器回归覆盖，不代表生产规模验证。
- 需要后续动作的事项：PR 需经人工最终审核；演练结果不代表真实发送、分桶或业务效果。
