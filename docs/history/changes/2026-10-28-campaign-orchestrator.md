# 决策记录：活动级编排器（活动流程）

关联：Snack/用户会话 2026-10-28（用户裁定需要编排器；案例问答后选定「活动级流程：编排器连多个投放项与边」并确认落盘）
承接 / 修订：事实 11、13、14、17、67；新增事实 68、69；同日稍早的「多动作三种表达」记录（`2026-10-28-multi-action-expression.md`）中「不引入编排器」表述被本裁定推翻

## 背景

运营需要并行、串行、按回执分支（如 RCS 成功→邮件、失败→短信）。首版曾写「用回流再建人群、不引入编排器」。用户明确：**这不对，需要编排器**；并在「投放项内多步」vs「活动级流程」案例中选定**活动级**。

## 关键实现（仅文档）

- **引入活动级编排器**（产品暂名「活动流程」；英文名未定）：活动下的流程对象，用**边**连接多个投放项；归属运营平台
- **边的条件**来自动作回执（成功 / 失败 / 契约其他结果）
- **一个投放项仍一个动作**（不推翻事实 52、64）
- **并行**：无边相连、同一入口可并行
- **回流 / 来源保留**（实验、圈人、跨活动复用），但**活动内串行 / 分支以边为准**，不再要求重建人群配置
- 修订事实 11、13、14、17、67；新增事实 **68、69**；名词表补「活动流程」「边」；确认记录与「由此推翻的判断」表补行
- `interaction-invariants`：名词 / 概念与关系 / 承接表最小交叉引用；**不另立不变量**
- `system-boundaries`：运营平台拆法与归属表补活动流程与边
- `AGENTS.md` 高风险第 1 条同步：事件与活动流程 / 边语义变更均属高风险
- 设计概览 HTML `#full-flow` 改为投放项 + 边示意，去掉「无编排器」标注
- **不改** `gmp-core`；**commit 不 push**

## 影响文件

- [`docs/domain/campaign-user-stories.md`](../../domain/campaign-user-stories.md)
- [`docs/domain/interaction-invariants.md`](../../domain/interaction-invariants.md)
- [`docs/domain/system-boundaries.md`](../../domain/system-boundaries.md)
- [`docs/design-overview-2026-10-27.html`](../../design-overview-2026-10-27.html)
- [`AGENTS.md`](../../../AGENTS.md)
- [`docs/domain/business-scenarios.md`](../../domain/business-scenarios.md)：场景表「事件串」措辞改为活动流程边（最小对齐）
- 本变更证据

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `python3 test/docs/consistency_check.py` | 见本轮执行 | 须全绿 |
| `gmp-core` / push | 未做 | 用户要求 commit 不 push |

## 风险与未覆盖项

- 真风险：界面若仍暗示「只能靠回流再建人群走下一步」或「一个投放项挂动作列表」，会与事实 68、69 / 52、64 冲突
- 本次不覆盖：活动流程 / 边的英文名（D2）；活动 / 投放项 / 流程运行态状态机（D1）；边是否支持「等待 N 天」等时间条件（未表态）
- 历史变更证据 `2026-10-28-multi-action-expression.md` 保留为首版记录；以本文件与事实 68、69 为准
