# 会话入口

本文件是 Claude Code 每次会话自动加载的入口文件，**本身不是规则真源**。

**唯一真源是仓库根目录 [AGENTS.md](AGENTS.md)，以及命中目录下的子级 `AGENTS.md`。如与 `AGENTS.md` 冲突，以 `AGENTS.md` 为准；如与当前会话用户指令冲突，以用户指令为准。**

## 每次会话开始时必须做

1. 读 [AGENTS.md](AGENTS.md) 全文，尤其是「AI Loop Native 约束」章节。
2. 进入任何代码 / 接口 / 配置修改前，先按 [任务入口检查清单](docs/ai-loop/task-intake-checklist.md) 做入口检查：读 `docs/history/memory/`（长期规则）+ 近期同领域 `docs/history/changes/`，整理成任务卡；未整理完不进入实施。
3. 涉及子目录改动时，同时读对应子级 `AGENTS.md`。
4. **禁止直接向 `main` 提交或推送**；所有变更走「issue → 分支 → PR → 人工放行」，流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。AI 不得自行合并 PR。

## 任务完成前必须做

- 评估器审查用 [评估器协议模板](docs/ai-loop/evaluator-protocol-template.md)，**主 Agent 不得自行宣布完成**。
- 评审通过、进入人工最终审核前，写一份 `docs/history/changes/`；是否需要补 `docs/history/memory/` 由评估器判断。

具体规则、分级治理、高风险清单、验收标准，全部以 `AGENTS.md` 为准，本文件不重复维护，避免产生不一致。
