# gmp-core README 分层状态修订实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `gmp-core/README.md` 准确描述当前已有的领域层实现和仍未覆盖的范围。

**Architecture:** 仅更新 README 的依赖分层状态与未覆盖项，不改代码或接口。逐项对照当前 Core 包、近期变更证据和根 `AGENTS.md` 中的业务回归覆盖状态，避免把“已有实现”写成“完整实现”。

**Tech Stack:** Markdown；不新增依赖。

**Spec:** `docs/superpowers/specs/2026-11-05-gmp-core-readme-status-design.md`

## Global Constraints

- 只改 `gmp-core/README.md`；任务卡、设计稿、计划与变更证据是流程产物。
- 不改 Go 代码、领域规格、接口契约、测试或依赖方向门禁。
- 保留“内存模型、不接真实外部链路”的项目边界。
- 不把活动流程实验节点的浏览器本地草稿描述成 Core 能力。
- 不声称各层完整实现或生产就绪；按根 `AGENTS.md` 如实说明业务场景覆盖仍有缺口。

## Review Focus

- 层状态描述的是代码与测试存在，不是对应规格已全部落地；人工逐项对照目录和变更记录。
- `scenarios` 的存在不代表所有业务场景已覆盖；对照根 `AGENTS.md` 的场景状态表。
- 活动流程实验节点仍未进入管理 API / Core；对照 2026-11-03 变更证据。
- 指标范围、真实存储与外部链路、时间与并发验证仍属未覆盖；对照 `system-boundaries.md` 和当前 README。
- 文档范围保持为状态修正与未覆盖项，不扩写成完整能力目录。

---

### Task 1: 修正 Core README 状态与范围

**Files:**
- Modify: `gmp-core/README.md`

**Interfaces:**
- Consumes: 当前 `gmp-core/{audience,campaign,experimentation,scenarios}/` 包及测试；`docs/history/changes/2026-11-01-activity-audience-entry-routing.md`；`docs/history/changes/2026-11-03-activity-flow-experiment.md`；根 `AGENTS.md` 业务回归覆盖表。
- Produces: 与当前主线一致的分层状态和剩余未覆盖项。

- [x] **Step 1: 更新分层状态表**

将已有的 `audience`、`campaign`、`scenarios` 状态从“未建立”修正为有范围限定的已实现状态；保留 `experimentation` 已建立状态。措辞不得暗示完整规格实现。

- [x] **Step 2: 修正未覆盖项**

删除把现有三层整体列为未建立的旧项；保留仍成立的存储、外部链路、指标、时间、并发和规格漂移检测边界，并说明活动流程实验节点尚未接入管理 API / Core。

- [x] **Step 3: 逐项复核差异**

人工对照上述来源，确认 README 的每条状态和缺口都有实现侧或规格侧依据；确认本任务未修改其他文件。

- [x] **Step 4: 检查文档差异**

运行 `git diff --check`，并人工检查 README 差异。文档任务不新增或运行代码测试。

- [x] **Step 5: 独立评估**

向只读评估器提供 Issue #84、任务卡、设计稿、README 差异和验证证据；等待其 `PASS`。主 Agent 不自行宣布评估通过。

- [x] **Step 6: 写入变更证据并准备 PR**

评估器 `PASS` 后，新增 `docs/history/changes/2026-11-05-gmp-core-readme-status.md`，记录本次事实修正、实际检查和未覆盖项；随后推送分支并创建关联 #84 的 PR，不合并。
