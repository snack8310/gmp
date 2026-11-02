# 变更记录

## 未发布

### 破坏性变更

- `management` 活动管理 `/api/v1` 原地调整：活动使用单个 `audience`（普通人群组或实验人群包）；`treatments[]` 移除 `audienceRef`；`activityFlow` 新增 `entries[]` 入口路由；受众与流程分别使用 `/audience`、`/flow` 写入。旧 `audienceRefs[]`、投放项级 `audienceRef`、`/audiences` 和旧流程请求体不再兼容。
- 活动实验臂现在显式路由到不同起始投放项或显式无投放对照；后续投放由活动回执边推进。核心 `gmp-core` 活动要求单一受众及完整活动流程。
- 影响范围：当前仓库内本机演示客户端与活动管理 API。该 V1 尚未合并，用户已确认原地更新、不增加 V2；没有生产调用方、持久化数据或迁移步骤。

