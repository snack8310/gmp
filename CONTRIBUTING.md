# 贡献指南

本文件只描述 GitHub 上的操作流程。**规则真源是根目录 [AGENTS.md](AGENTS.md)**，如有冲突以 `AGENTS.md` 为准。

## 流程

1. **建 issue**。从 `.github/ISSUE_TEMPLATE/` 选对应模板（缺陷 / 需求 / 重构与文档）。issue 是任务卡的输入，不是任务卡本身。
2. **建分支**：`<type>/<issue 编号>-<短横线主题>`，例如 `fix/34-duplicate-delivery-on-retry`。**不要直接在 `main` 上改。**
3. **入口检查**。进入任何文件修改前，按 [任务入口检查清单](docs/ai-loop/task-intake-checklist.md) 产出任务卡；未整理完不进入实施。
4. **实施并自测**。按任务卡的最小验证集执行，记录真实命令结果。
5. **评估器审查**。按 [评估器协议模板](docs/ai-loop/evaluator-protocol-template.md) 发起独立只读评估器审查。**实施方不得自行宣布完成。**
6. **写变更证据**，落到 `docs/history/changes/`；是否新增 `docs/history/memory/` 由评估器判断。
7. **提 PR**，填完 `.github/pull_request_template.md`，关联 `Closes #N`。
8. **人工放行**。合并由人工执行，AI 不得自行合并。

## Commit

Conventional Commits，英文书写：

```
<type>(<scope>): <subject>

<body>

Refs #<issue 编号>
```

破坏性变更用 `feat(api)!: ...` 形式，并在 body 说明影响范围。

## 提交前自查

- [ ] 未直接向 `main` 提交
- [ ] PR 关联了 issue
- [ ] 评估器结论为 `PASS`
- [ ] 变更证据已写入
- [ ] 无内部主机名、内部服务名、工单号、凭据或真实业务数据

详细的分级治理、高风险卡点、验收标准见 [AGENTS.md](AGENTS.md)。
