# 决策记录：受众名词改写（人群配置 / 人群组）

关联：Snack 会话 2026-10-27（用户确认 gmp 受众名词改写）；承接 [`2026-10-27-experiment-audience-bundle-aq.md`](2026-10-27-experiment-audience-bundle-aq.md) 与 [`2026-10-27-reusable-experimental-audience.md`](2026-10-27-reusable-experimental-audience.md)

## 背景

「人群定义」作主名时，运营与文档里常把它跟「来源」混谈；对外挂接（活动 / 投放项挂什么）与求值对象（来源 + 筛选）也未分清。用户已拍板三点，须在 `docs/domain/` 落盘并与实验人群包 / AQ1–AQ3 对齐，关闭命名歧义。机制与边界**不变**。

## 关键实现（仅文档）

- **人群配置** = 来源 + 筛选（筛人是配置的一部分）；不再以「人群定义」当主名。
- **人群组** = 受众对外挂接：无实验组数 = 1；有实验 = 多臂多组。
- **实验人群包**（`ExperimentAudienceBundle`）与 AQ1–AQ3 结论保留：包发布的是多个人群组；每臂内部仍是带「第 N 份」条件的人群配置。
- 写入新旧映射表、受众侧事实 26、确认记录；修正事实 9 用语；同步活动侧 / 边界 / 不变量 / 场景文档主名。
- **不改** `gmp-core`（代码层仍为 `audience.Definition`）、不 push / commit（除非另说）。

### 新旧名词映射

| 旧说法 / 易混说法 | 现主名 | 说明 |
|---|---|---|
| 「人群定义」`AudienceDefinition` | **人群配置** `AudienceConfig`（建议） | 同一求值对象改主名；= 来源 + 筛选。代码层现为 `audience.Definition` |
| 旧名词表「待命名的筛选配置」空行 | **人群配置** | 空行闭合 |
| 口语「用户组 / 名单」在对外挂接义 | **人群组** `AudienceGroup`（建议） | 无实验 → 1 组；有实验 → 多臂多组 |
| 「可复用实验人群」作为发布单元 | **实验人群包** `ExperimentAudienceBundle` | AQ1 已确认；包内各臂 = 人群组 |
| 「来源」 | **来源**（不变） | 只表示能吐 uid 的供给方，不含筛选 |

## 关键文件

- [`docs/domain/audience-user-stories.md`](../../domain/audience-user-stories.md)：名词表 + 映射；能力 / Q / 事实用语；事实 9 再修正；事实 26；确认记录 · 受众名词改写。
- [`docs/domain/campaign-user-stories.md`](../../domain/campaign-user-stories.md)：投放项与受众关系节；条件分流 / 决策表 / 事实 10、17、22、58 等主名。
- [`docs/domain/system-boundaries.md`](../../domain/system-boundaries.md)：拆法图、接缝、归属表（人群配置 / 人群组）。
- [`docs/domain/interaction-invariants.md`](../../domain/interaction-invariants.md)：名词表、概念与关系、承接表补受众侧事实 26。
- [`docs/domain/business-scenarios.md`](../../domain/business-scenarios.md)：场景 ⑤ 用语；「多个活动共用」改为故事 15 `已确认`。
- 本变更证据。此前两份 2026-10-27 变更证据不回改（时点记录）。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `python3 test/docs/consistency_check.py` | 见本轮执行 | 须全绿（事实编号、跨文档引用、设计文档承接表等） |
| `gmp-core` 代码 diff | 应为空 | 本轮非目标 |
| push / merge | 未做 | 用户明确要求 |

## 风险与未覆盖项

1. 英文名 `AudienceConfig` / `AudienceGroup` 为建议拼写，与 `ExperimentAudienceBundle` 的 D9 同类；代码标识仍为 `Definition`，改名需另开代码任务。
2. `AGENTS.md`、任务卡、前端 demo、`gmp-core` 注释仍可能出现旧主名「人群定义」——本轮只改 `docs/domain/` + 本变更证据，避免扩大范围。
3. 历史变更证据与确认记录中的用户原话保留「人群定义」字样，属时点忠实，不追溯改写。

## 需要后续动作的事项

- 用户若确认或改掉 `AudienceConfig` / `AudienceGroup` 英文拼写，再同步名词表与（届时）代码标识。
- 若要将 AGENTS / demo / core 注释一并改主名，另开任务，勿与本轮混做。
