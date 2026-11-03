# 实验列表与详情实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把实验入口做成可深链的列表与独立详情，并让比例及模拟染色操作在浏览器构造数据中生效。

**Architecture:** `audience.mjs` 仍持有本地实验数据、缓存版本和纯分配函数；实验页面从同一 store 读取、写入，受众预览复用只读分配方法。`app.mjs` 的哈希路由分发实验列表、新建与详情，页面不调用管理 API 或 Core。

**Tech Stack:** 原生 JavaScript ESM、浏览器 `localStorage`、Node 内置测试、现有静态服务器与离线导出。

**Spec:** [实验列表与详情设计](../specs/2026-11-03-experiment-list-detail-design.md)

## Global Constraints

- 只用构造 `SIM-*` ID；比例为整数百分比，A/B/对照均在 0–100 且合计 100。
- 染色策略创建时必选；运行后不可切换；运行中比例调整不推进代际。
- 旧缓存先验证、备份再升级；读失败不覆盖原值。
- 比例和染色仅为本地演示；不修改管理 API 与 `gmp-core`。
- 受众预览只读；实验分配写入须是独立用户动作。

## Review Focus

- 同一构造 ID 在详情与受众预览中应落同一臂；任务一和任务三增加跨页断言。
- 非法比例保存后旧状态应完整保留；任务一增加快照比较。
- 旧版缓存升级失败时原字节应不变；任务一增加存储失败反例。
- 运行中改比例不应让已发布包失效或移动旧染色；任务一增加代际和旧值断言。
- 快速切换实验详情时过期 UI 操作不得改另一个实验；任务二增加路由切换回归。

---

### Task 1: 本地实验数据与迁移

**Files:** `frontend/demos/operations-flow/audience.mjs`、`audience.test.mjs`。

**Interfaces:** `createAudienceStore(storage)` 增加 `peekExperimentGroup(id, uid)`、`assignExperimentGroup(id, uid)`、`updateExperimentSplit(id, split)`、`experimentColoring(id)`；`saveExperiment` 的新建输入增加 `split` 与 `colored`。`split` 为 `{a,b,control}` 整数百分比；染色记录为构造 ID 到臂的映射。

- [ ] 先写失败测试：比例验证、创建必须选择染色、只读零写入、首次模拟分配后改比例旧臂不变且新用户用新比例、运行态与包代际、旧缓存迁移/备份/失败关闭。
- [ ] 运行 `node --test frontend/demos/operations-flow/audience.test.mjs`，确认新增用例失败。
- [ ] 实现缓存新版本、严格校验、迁移、备份及上述 store 方法。原 `assignGroup` 改为消费明确 split 的纯函数；旧字段只按旧固定行为迁移。
- [ ] 重跑该测试，确认通过；提交 `feat(experiment): add local split and coloring state`，正文说明本地模拟边界，引用 #80。

### Task 2: 列表、详情与新建页面

**Files:** `activity-routing.mjs`、`activity-routing.test.mjs`、`app.mjs`、`experiment-ui.mjs`、`index.html`、`experiment-page.css`（新建）、`serve.mjs`、`export.mjs`、相关前端测试。

**Interfaces:** `parseActivityRoute` 为实验路由返回 `assetView: 'list'|'detail'|'new'` 及 `experimentId`；`mountExperiments` 返回 `render(route)`，所有列表和详情从 Task 1 的 store 取值。

- [ ] 先写失败测试：实验深链、未知 ID、列表筛选、创建后进入详情、比例编辑、染色配置不可变、刷新与历史导航、快速切换实验后旧操作失效、离线导出包含页面资产。
- [ ] 运行对应路由与页面测试，确认新增用例失败。
- [ ] 实现路由、页面与独立样式，沿用活动列表/详情视觉；展示模拟染色样本和包引用，保留 `return`；HTML/静态服务器/导出白名单同步。
- [ ] 重跑路由、页面、导出测试，确认通过；提交 `feat(experiment): add routed list and detail pages`，引用 #80。

### Task 3: 跨页面口径与文案

**Files:** `audience-ui.mjs`、`app.test.mjs`、`frontend/demos/operations-flow/README.md`、`CHANGELOG.md`。

**Interfaces:** 受众预览调用 Task 1 的 `peekExperimentGroup`，不再直接以固定比例调用 `assignGroup`；实验详情与受众页使用同一 store。

- [ ] 先写失败测试：实验比例变更后受众预览的臂统计同步；预览不写染色；保存失败提示当前会话边界；旧缓存升级后旧页面不可读的回滚说明在文档中存在。
- [ ] 运行前端测试，确认新增用例失败。
- [ ] 接通受众预览，移除固定比例文案，更新 README、CHANGELOG 与兼容说明。
- [ ] 运行前端测试、导出检查、语法检查和文档门禁；提交 `feat(experiment): align audience preview with local experiment`，引用 #80。

### Task 4: 浏览器验收与独立评估

**Files:** `docs/history/changes/2026-11-03-experiment-list-detail.md`，必要时仅修复本任务发现的缺陷。

- [ ] 本机打开桌面与窄屏，走列表→详情→调整比例→模拟分配→再次调整→刷新→返回；保存截图证据。
- [ ] 运行完整前端测试、`node --check`、离线导出检查、`python3 test/docs/consistency_check.py`、`git diff --check`；记录真实命令和未覆盖项。
- [ ] 以任务卡、diff、设计和验证结果请求独立只读评估器；只在 `PASS` 后写变更证据并进入人工最终审核。
- [ ] 同步 issue 与 PR 的风险、兼容、验证和评估结论；不自动合并。
