# 实施计划：活动受众入口与流程路由模型

关联：[#78](https://github.com/snack8310/gmp/issues/78)。工作分支：`feat/78-campaign-entry-routing`，基于尚未合并的 `feat/76-activity-flow-canvas`。

## 1. 补齐任务测试矩阵（先写失败测试）

- 为 `audience.Selection` 写测试：普通组与单个实验包可构造；零值与未发布包拒绝；能枚举包内每个臂且顺序稳定。
- 为 `campaign.Flow` 和 `Campaign` 写测试：必需入口/流程；普通组入口；实验包臂完整路由；显式对照无投放（不用长期 Holdout 语义）；不重叠的臂路径；无入口路由、重复分区、非所选组、未知节点、孤立节点拒绝。
- 把活动内成功/失败分支场景改为 activity-level entry；目标投放项仍引用一份非空且与回执 UID 不相交的条件组，断言只对回执 UID 执行。
- 为共享活动决策写失败测试：入口人群必须先限缩；决策表只能从该组入口候选项中选择；业务场景③的新客/老客路由保持；活动放量仍全臂一致。

## 2. 迁移 `gmp-core/audience` 与 `gmp-core/campaign`

- 新增封闭 `audience.Selection` 与服务端校验/按 UID 解析入口分区的方法。
- `CampaignSpec` 持有唯一 Selection 和必需 Flow；`TreatmentSpec` 删除 `Who` 与 `Treatment.Audience()`。
- `FlowSpec` 加入口路由、显式实验对照无投放状态；保留 receipt edge 和 parallel edge 的现有行为。
- 迁移活动服务、实时展示决策、枚举执行、演练与跨层场景；决策表条件作为所选活动受众内的条件。
- 更新所有调用点与测试；不修改 `experimentation` 的分桶、染色或指标语义。

## 3. 迁移活动管理 API V1

- 用单值 `audience` 取代 `audienceRefs[]`；支持 group / bundle 选择。
- 从 `Treatment` 删除 `audienceRef`；给 `ActivityFlow` 添加入口路由；对照显式标记为 `control`。
- 活动受众以并发冲突保护写入；更换受众时服务端原子清空旧入口路由、保留回执边。再通过流程写接口并发保护保存入口路由与回执边；未完成入口的草稿不可演练。
- 更新 store 校验、种子与 HTTP 测试。活动空草稿可继续存在，但不完整流程不得开始本地演练。

## 4. 对齐现有前端 3D 演示

- 移除投放项表单的人群选择器，以活动级单选器代替。
- 在现有 3D SVG 流程画布内显示活动入口；实验包按臂连到起始投放项，对照臂显示为无投放；后续边保持回执连线。实时展示决策只呈现当前活动入口臂允许的候选起点。
- 复用原画布 renderer、拖拽/旋转/演练逻辑及独立画布 CSS；不重写 SVG，不触碰核心样式表。
- 更新 API client、活动样例、浏览器检查与说明；同步清理旧 V1 字段。

## 5. 对齐规格与破坏性变更记录

- 更新 `campaign-user-stories.md`、`interaction-invariants.md`、`system-boundaries.md` 和活动 API 文档；把新模型关联到当前用户确认。
- 添加 `CHANGELOG.md` 条目，明确 V1 的 `audience`、flow entries 变化、移除的字段及调用方范围。
- 更新 `docs/history/changes/` 变更证据，记录风险验证与未覆盖项。

## 6. 验证及独立评估

- `gmp-core/`: `go test ./...`。
- `management/`: `go test ./...`、`go build -o /tmp/gmp-entry-routing-api ./cmd/campaign-api`。
- 演示：`node --test --test-reporter=dot frontend/demos/operations-flow/*.test.mjs` 与项目既有 browser-check；确认 3D 画布保留且 API 旧字段已清理。
- 规格：`python3 test/docs/consistency_check.py`、`git diff --check`。
- 提供任务卡、领域记忆、需求规格、实际 diff 与命令结果给独立只读评估器；记录 `PASS` / `FAIL` / `NEEDS-EVIDENCE`，最多按大任务规则修复三轮。
