# 决策记录：关闭 AQ1–AQ3，实验人群包为一等公民

关联：Snack 会话 2026-10-27（回答 widget t42s2）；承接 [`2026-10-27-reusable-experimental-audience.md`](2026-10-27-reusable-experimental-audience.md)

## 背景

上一轮把「人群侧可复用实验人群」写入决策记录，但 AQ1–AQ3（是否一等公民、分流修订后臂如何处置、跨活动指标聚合口径）与设计文档 D7、D8 仍开放。用户已拍板三点，须落盘并关掉开放问题，否则实现会在「松散约定 vs 发布单元」上分叉。

## 关键实现（仅文档）

- **AQ1**：另立一等公民——分流与各臂**打包发布**；中文「实验人群包」，英文建议 `ExperimentAudienceBundle`（拼写待命名确认，见 D9）。
- **AQ2**：分流修订 / 停用 → **强制停用**相关臂（及包可用状态）；落成不变量 20 与状态机。
- **AQ3**：按分流臂跨活动聚合 **与** 按活动切开 **两种都要**；落成不变量 21、活动侧事实 61。
- **修正受众侧事实 9**：求值入口仍是人群定义；发布单元另立实验人群包。
- **归属表**增加实验人群包一行（归受众服务）；分流本体仍在实验服务。
- **不改** `gmp-core`、不 push / merge。

## 关键文件

- [`docs/domain/audience-user-stories.md`](../../domain/audience-user-stories.md)：名词与 A9/A10；确认记录 · AQ1–AQ3；事实 9 修正、23–25；AQ1–AQ3 结论表。
- [`docs/domain/campaign-user-stories.md`](../../domain/campaign-user-stories.md)：分流节用语；事实 61。
- [`docs/domain/system-boundaries.md`](../../domain/system-boundaries.md)：接缝硬边界表与跨边界归属表。
- [`docs/domain/interaction-invariants.md`](../../domain/interaction-invariants.md)：名词表、概念、不变量 20–21、状态机、承接表、D7/D8 关闭与 D9。
- 本变更证据。上一轮 [`2026-10-27-reusable-experimental-audience.md`](2026-10-27-reusable-experimental-audience.md) 不回改（时点记录）。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `python3 test/docs/consistency_check.py` | 见本轮执行 | 须全绿 |
| `gmp-core` / push / merge | 未做 | 用户要求 |

## 风险与未覆盖项

1. 英文拼写仅建议（D9）；若日后改名需同步名词表与代码标识。
2. 实验人群包的完整字段 / 发布 API 未定——本轮只定语义与生命周期要点。
3. 不变量 21 的「同时暴露两种聚合」落在哪一侧查询入口（实验服务 vs 运营平台）未细拆，执行记录带投放项+份号（不变量 15）仍是共同前提。
4. 故事 19–22（改名单与停名单）仍多为 `待确认`；AQ2 只定了「包随分流停」，未定活动侧停用 UI 的全部细节。

## 需要后续动作的事项

- 用户若确认或改掉英文名，关闭 D9。
- 代码落地前再开任务卡：包的构造校验、停用传播、双视图指标。
- #44 仍按既有决定暂不动，除非另说。
