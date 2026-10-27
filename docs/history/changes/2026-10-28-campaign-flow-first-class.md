# 决策记录：活动流程为一等公民

关联：Snack/用户会话 2026-10-28（用户确认：活动流程应进一等公民清单；正式名用「活动流程」，不要只叫「流程」；落盘后提交并 push）
承接：事实 68、69（活动级编排器）；新增事实 70；强化事实 68 一等公民措辞

## 背景

活动级编排器已写入（事实 68、69），但名词表仍写「产品暂名」，未明确与动作 / 来源 / 实验人群包同级的**一等公民**地位。用户裁定正式中文名「活动流程」，并要求写入一等公民清单后提交仓库。

## 本次实现（仅文档）

- **正式中文名**：活动流程（勿只称「流程」）；编排器为其产品说法；英文名仍未定（D2）
- **一等公民**：与动作（`Action`）、来源、实验人群包同级
- 事实 **68** 强化一等公民措辞；新增事实 **70**（一等公民清单）
- 名词表、确认记录；`system-boundaries` 归属行；`interaction-invariants` 名词 / 概念 / 承接表
- 设计概览 HTML：一等公民清单 + 运营平台分层与流程示意措辞
- **不改** `gmp-core` 代码模型

## 影响文件

- [`docs/domain/campaign-user-stories.md`](../../domain/campaign-user-stories.md)
- [`docs/domain/system-boundaries.md`](../../domain/system-boundaries.md)
- [`docs/domain/interaction-invariants.md`](../../domain/interaction-invariants.md)
- [`docs/design-overview-2026-10-27.html`](../../design-overview-2026-10-27.html)
- 本变更证据

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `python3 test/docs/consistency_check.py` | 见本轮执行 | 须全绿 |
| `git push origin main` | 见本轮执行 | 用户要求 push |

## 风险与未覆盖项

1. 活动流程 / 边的英文名仍未定（D2）
2. 活动 / 投放项 / 活动流程运行态状态机仍未表态（D1）
3. 边是否支持「等待 N 天」等时间条件仍未表态
