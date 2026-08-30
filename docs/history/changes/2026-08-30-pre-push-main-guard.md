# pre-push 钩子拦截推向受保护分支

关联：issue #2

## 背景

`AGENTS.md`「协作流程与分支约定」规定「禁止直接向 `main` 提交或推送」，但该约束此前只存在于规则文档层 —— 按仓库自己定义的「约束落地优先级」，这是最弱的一层。

远端 Ruleset `protect-main` 已提供强制力（`bypass_actors: []`，owner 亦无法绕过），但拦截发生在 push 之后：错误提交已经落在本地 `main` 上，需要 reset 回退，且错误信息来自 GitHub 服务端，不指向仓库自己的规范。本次把拦截时机前移到 push 之前。

## 关键实现

- 新增 `.githooks/pre-push`：解析 git 传入的每行 `<local ref> <local sha> <remote ref> <remote sha>`，命中 `main` / `master` 时以退出码 1 拒绝，并输出指向 `CONTRIBUTING.md` 与 `AGENTS.md` 的中文提示。
- 分支删除（local sha 全 0）、tag 推送、非分支 ref 一律放行。分支名按**字面量精确匹配**，不做子串匹配。
- 钩子进版本库（`.githooks/`）而非 `.git/hooks/`，以便被 review；通过 `git config core.hooksPath .githooks` 启用，说明写入 `CONTRIBUTING.md`。
- 评估器首轮审查后修复两处可移植性缺陷：分支列表由变量遍历改为字面量 `case` 模式（原写法在 zsh 下拦截完全失效）；`while read` 增加 `|| [ -n "$local_ref" ]`（原写法丢弃无换行符结尾的末行）。

## 关键文件

- `.githooks/pre-push`：新增，55 行。
- `CONTRIBUTING.md`：新增「启用本地钩子」一节。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| 最小验证集 10 组 stdin 用例断言退出码 | 通过 | main=1、master=1、多 ref 混合=1；特性分支 / 删除分支 / tag / `feat/maintain-x` / `domain-fix` / `main-backup` / 空 stdin 均为 0 |
| 5 种解释器交叉执行（sh/bash/dash/zsh/ksh） | 通过 | 修复后全部一致；修复前 zsh 下推 main 退出 0（缺陷已闭合） |
| 末行无换行符 | 通过 | 修复后退出 1；修复前退出 0 |
| glob 误匹配用例（`*`、`?ain`、`ma*n`、`Main`、`mainx`、`foo/main`） | 通过 | 全部放行 |
| 端到端 `git push --dry-run origin HEAD:main` | 通过 | git 实际调用钩子并拦截，退出码 1，未发出 ref 更新 |
| 端到端 `git push --dry-run` 推特性分支 | 通过 | 正常放行，退出码 0 |
| 独立评估器审查（两轮） | PASS | 首轮 PASS + 2 条非阻断风险；实施方修复后复审 PASS，无回归 |

## 风险与未覆盖项

- **真风险（放行后续跟进）**：钩子可被 `--no-verify` 绕过，且需每个 clone 手工执行一次启用命令。这是设计取舍 —— 它防手滑与 AI 误操作，不防有意为之；强制力仍由远端 Ruleset 承担。两层不是替代关系。
- **真风险（放行后续跟进）**：仓库中没有针对该钩子的可执行回归用例，「不要退回变量遍历」目前只靠脚本注释约束，属规则文档层。已开 issue #3 承接自动化门禁。
- **本次不覆盖项**：Windows / 非 POSIX 环境；busybox ash、mksh 等解释器；本地在 `main` 上 commit 的拦截（pre-commit 层，属非目标）；删除 `main`（`git push origin :main`）不被本地拦截，由远端 Ruleset 拒绝。
