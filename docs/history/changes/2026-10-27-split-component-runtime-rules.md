# 决策记录：分流组件运行时规则

关联：Snack 会话 2026-10-27（用户确认写入 gmp docs）；承接 [`2026-10-27-experiment-audience-bundle-aq.md`](2026-10-27-experiment-audience-bundle-aq.md)、[`2026-10-27-audience-naming-rewrite.md`](2026-10-27-audience-naming-rewrite.md)

## 背景

实验人群包 / AQ2 已定「分流修订或停用 → 强制停用臂」，但**运行中**还能改什么、**非运行中**还能不能被活动挂上，未写清。若不补，实现会在「比例调整是否算修订」「停用后的分流能否被新活动引用」上分叉，且与「定义不可就地修改」旧读法冲突。

## 关键实现（仅文档）

- **分流组件**（产品说法；本体 = 实验服务一次分流定义）：经实验人群包发布后，各臂对外为**人群组**，求值走**人群配置**。
- **运行中**（无论是否已被活动引用）：可调**比例**、**白名单**等运行参数；**不可改「加载组件逻辑」**。
- **非运行中**：不可被其他活动引用。
- **已修正** `interaction-invariants.md` 中「一次分流」状态机：运行参数就地调 ≠ 强制新修订；加载组件逻辑变更仍须非运行后修订；与 AQ2 / 不变量 20 衔接。
- 新不变量 22、23；受众侧事实 27、28；活动侧事实 62；能力 A11。
- **不改** `gmp-core`、不 push / commit（除非另说）。

## 关键文件

- [`docs/domain/audience-user-stories.md`](../../domain/audience-user-stories.md)：名词；A11；事实 27、28；确认记录。
- [`docs/domain/campaign-user-stories.md`](../../domain/campaign-user-stories.md)：名词「分流组件」；运行时规则小节；事实 62。
- [`docs/domain/system-boundaries.md`](../../domain/system-boundaries.md)：归属表一行；染色节与运行态关系说明。
- [`docs/domain/interaction-invariants.md`](../../domain/interaction-invariants.md)：名词；概念；不变量 22–23；状态机**已修正**；承接表。
- 本变更证据。此前同日变更证据不回改。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `python3 test/docs/consistency_check.py` | 见本轮执行 | 须全绿 |
| `gmp-core` / push | 未做 | 用户要求 |

## 风险与未覆盖项

1. 「加载组件逻辑」英文名未定；切法份数增减是否一律算加载逻辑、白名单的数据结构未细拆。
2. 运行中调比例时，未染色键是否立即按新比例重算——机制仍见染色表，本轮不新开算法事实。
3. `gmp-core` 仍按旧「定义不可就地修改」实现；代码对齐另开任务。

## 需要后续动作的事项

- 代码任务：运行态校验、运行参数写入路径、非运行引用拦截。
- 若用户给出「加载组件逻辑」英文名或更细白名单规则，再补名词表。
