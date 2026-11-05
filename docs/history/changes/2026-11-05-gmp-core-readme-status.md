# 修正 gmp-core README 分层状态

## 背景

Issue #84 指出，`gmp-core/README.md` 仍将已有的 `audience`、`campaign`、`scenarios` 层标为未建立，且把它们整体列入未覆盖范围，和当前代码及近期变更记录不一致。

## 关键实现

- 更新 README 分层状态，区分已有内存模型、稳定分配与染色，以及仍未覆盖的跨层业务场景。
- 明确活动流程实验节点仍是浏览器本地草稿，尚未进入管理 API / `gmp-core` 或真实执行。
- 保留真实存储、外部链路、指标范围、时间与并发验证等未覆盖边界。

## 关键文件

- `gmp-core/README.md`：校正分层状态及未覆盖说明。
- `docs/ai-loop/task-cards/84-gmp-core-readme-status.md`：记录入口检查与验收范围。
- `docs/superpowers/specs/2026-11-05-gmp-core-readme-status-design.md`：记录获批的最小修正方案。
- `docs/superpowers/plans/2026-11-05-gmp-core-readme-status.md`：记录实施步骤与验证边界。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| `git diff HEAD^ HEAD --check` | 退出码 0 | README 提交差异无空白错误。 |
| `python3 -B test/docs/consistency_check.py` | 退出码 0 | 独立只读评估器执行，文档一致性检查通过。 |
| 独立只读评估器 | `PASS` | 审查提交 `42c7cf5608b93b7fe6a2be5c36ba68cc32363ceb`，无阻断问题；指出一项既有时间 / 并发措辞的非阻断歧义。 |

## 风险与未覆盖项

- 真风险（仅观察）：评估器指出，README 既有的时间与并发概述可能被读成完全没有相关场景或契约用例；该措辞不由本次变更引入，本次最小范围未调整。
- 本次不覆盖项：领域规格完整性审计、真实存储与渠道、指标口径、时间与并发的生产实现验证，以及活动流程实验节点的管理 API / Core 接入。
- 需要后续动作的事项：无。本次没有代码变更；主代理未运行代码测试。
