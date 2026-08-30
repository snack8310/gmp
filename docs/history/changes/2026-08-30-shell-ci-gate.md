# 建立 shell 脚本自动化门禁

关联：issue #3（前置：issue #2）

## 背景

issue #2 修复的缺陷类型是**静默失效**：`.githooks/pre-push` 用 `for x in $VAR` 遍历分支列表，zsh 不对未加引号变量做词分割，导致拦截逻辑完全不生效 —— 但脚本不报错、退出码 0、单解释器测试全过。该缺陷靠人工对抗性审查发现。

修复后新增的约束写在 `docs/history/memory/shell-script-portability.md` 里，属于「约束落地优先级」中最弱的规则文档层。两轮评估器独立指出了同一件事：这部分本可以固化为自动化检查。本次即该下沉动作。

## 关键实现

- `test/githooks/pre-push_test.sh`：POSIX sh 编写，向钩子喂 stdin 并断言退出码，覆盖拦截路径与放行路径。遍历 `sh` / `bash` / `dash` / `zsh` / `ksh`，缺失的解释器报告为跳过；**一个解释器都没跑到时判为失败**，避免「什么都没跑却显示绿灯」。
- 用例覆盖记忆文档要求的三组最短触发路径：zsh 解释器、末行无换行符、分支名含通配符字符。
- `.github/workflows/shell.yml`：本仓库首个 CI。在 push 到 `main` 与所有 PR 上运行用例 + `shellcheck -s sh`。显式声明 `permissions: contents: read`，不使用任何 secret，除官方 `actions/checkout` 外不引入第三方 action。
- `docs/history/memory/shell-script-portability.md`：新增「落地层级」一节，说明规则已由哪些检查承担，文档只保留「为什么这么写」。

## 关键文件

- `test/githooks/pre-push_test.sh`：新增，100 条断言。
- `.github/workflows/shell.yml`：新增。
- `docs/history/memory/shell-script-portability.md`：更新落地层级说明。
- `.githooks/pre-push`：仅新增两行 shellcheck 指令与注释，**行为未变**。

## 验证结果

| 命令 / 检查 | 结果 | 证据摘要 |
|---|---|---|
| 本地 `sh test/githooks/pre-push_test.sh` | 通过 | 100 断言，跑到 sh/bash/dash/zsh/ksh 五种解释器 |
| **变异测试 A**：钩子改回变量遍历 | **用例失败 5/100** | 全部为 zsh 下的拦截用例，精确复现 issue #2 缺陷 |
| **变异测试 B**：去掉 EOF 兜底 | **用例失败 5/100** | 「末行无换行」用例，五种解释器各一 |
| **变异测试 C**：改成子串匹配 | **用例失败** | 误拦 `feat/maintain-x` / `domain-fix` / `main-backup` / `mainx` / `foo/main` |
| 三次变异后还原 | 通过 | `git diff .githooks/pre-push` 为空 |
| CI 首次运行（run 33292918671） | **失败** | shellcheck 报 SC2034（`remote_sha` 未使用）与 SC1007（`CDPATH= cd` 被读作误写赋值）—— 门禁生效 |
| CI 修复后运行（run 33292976057） | **通过** | 五种解释器就位；100 断言全过；shellcheck 0.9.0 通过 |

## 风险与未覆盖项

- **真风险（仅观察）**：CI 只在 ubuntu runner 上运行，本地开发者仍可能在其他环境写出不可移植代码。缓解手段是测试脚本本身可在本地直接执行。
- **真风险（仅观察）**：`shellcheck -s sh .githooks/* test/githooks/*.sh` 的 glob 是显式路径，新增 shell 脚本时需手工纳入。已在记忆文档中写明。
- **本次不覆盖项**：Windows；busybox ash、mksh；非 ubuntu runner；`shellcheck` 版本差异导致的规则漂移。
- **落地层级残余**：记忆文档规则 1（词分割）可由 shellcheck SC2086 静态捕获，规则 2（EOF 兜底）shellcheck 覆盖不到，只能靠用例 —— 两者缺一不可，已在文档中写明。
