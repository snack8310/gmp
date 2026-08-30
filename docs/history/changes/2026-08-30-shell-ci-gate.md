# 建立 shell 脚本自动化门禁

关联：issue #3（前置：issue #2）

## 背景

issue #2 修复的缺陷类型是**静默失效**：`.githooks/pre-push` 用 `for x in $VAR` 遍历分支列表，zsh 不对未加引号变量做词分割，导致拦截逻辑完全不生效 —— 但脚本不报错、退出码 0、单解释器测试全过。该缺陷靠人工对抗性审查发现。

修复后新增的约束写在 `docs/history/memory/shell-script-portability.md` 里，属于「约束落地优先级」中最弱的规则文档层。两轮评估器独立指出了同一件事：这部分本可以固化为自动化检查。本次即该下沉动作。

## 关键实现

- `test/githooks/pre-push_test.sh`：POSIX sh 编写，向钩子喂 stdin 并断言退出码，覆盖拦截路径与放行路径。遍历 `sh` / `bash` / `dash` / `zsh` / `ksh`，缺失的解释器报告为跳过；**一个解释器都没跑到时判为失败**，避免「什么都没跑却显示绿灯」。注意 ubuntu 上 `/usr/bin/sh` 是 dash 的软链，因此这五个候选实际是**四种不同实现**（dash、bash、zsh、ksh）。
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
| 本地 `sh test/githooks/pre-push_test.sh` | 通过 | 105 断言，跑到 sh/bash/dash/zsh/ksh 五个候选（四种不同实现） |
| **变异测试 A**：钩子改回变量遍历 | **用例失败 5/105** | 全部为 zsh 下的拦截用例，精确复现 issue #2 缺陷 |
| **变异测试 B**：去掉 EOF 兜底 | **用例失败 5/105** | 「末行无换行」用例，五种解释器各一 |
| **变异测试 C**：改成子串匹配 | **用例失败** | 误拦 `feat/maintain-x` / `domain-fix` / `main-backup` / `mainx` / `foo/main` |
| 三次变异后还原 | 通过 | `git diff .githooks/pre-push` 为空 |
| CI 首次运行（run 33292918671） | **失败** | shellcheck 报 SC2034（`remote_sha` 未使用）与 SC1007（`CDPATH= cd` 被读作误写赋值）—— 门禁生效 |
| CI 修复后运行（run 33292976057） | **通过** | 五个解释器候选就位；断言全过；shellcheck 0.9.0 通过 |
| **变异测试 E**：删掉全零 sha 判断（评估器发现） | **用例失败 5/105** | 修复前该变异**无法被捕获**，见下 |
| 评估器独立审查 | PASS | 另做变异 D/D2/F，并补验 ksh 包名、shellcheck 不可用时不静默跳过 |

## 评估器发现的空断言（已修复）

评估器对**测试本身**做了变异测试，发现 `"delete a remote branch (all-zero local sha)"` 用例的输入取的是 `refs/heads/old` —— 该分支本就不受保护，两层判断都会放行，因此该断言**从未触碰全零 sha 判断**。把 `.githooks/pre-push` 的整块全零判断删除后，套件仍 100/100 全绿。

修复：该用例改为指向 `refs/heads/main`（删除受保护分支才是真正会走到全零判断的路径），并保留原 `refs/heads/old` 用例。断言数 100 → 105，变异 E 现可捕获（5/105 失败）。

**教训与 issue #2 同构**：测试自身也会静默失效。用例的输入必须取在能触发被测分支的取值上，而不是取在「名字听起来对」的取值上。

## 风险与未覆盖项

- **真风险（仅观察）**：CI 只在 ubuntu runner 上运行，本地开发者仍可能在其他环境写出不可移植代码。缓解手段是测试脚本本身可在本地直接执行。
- **真风险（放行后续跟进）**：`shellcheck -s sh .githooks/* test/githooks/*.sh` 的 glob 是显式路径，不匹配子目录，新增 shell 脚本会被静默漏检。评估器指出这本可用 `git ls-files` 固化为第 2 层。已开 issue #6。
- **真风险（放行后续跟进）**：变异测试目前每次都靠人工/评估器手工复现，未固化。评估器判定应做成脚手架并纳入同一 workflow。已开 issue #6。
- **本次不覆盖项**：Windows；busybox ash、mksh；非 ubuntu runner；`shellcheck` 版本差异导致的规则漂移。
- **落地层级残余**：记忆文档规则 1（词分割）可由 shellcheck SC2086 静态捕获，规则 2（EOF 兜底）shellcheck 覆盖不到，只能靠用例 —— 两者缺一不可，已在文档中写明。
