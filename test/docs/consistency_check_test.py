#!/usr/bin/env python3
"""Mutation tests for the docs consistency gate.

A check that finds its own subjects can pass by finding none: rename a
directory or a heading and the loop body never runs while the exit code stays
zero. Every check in the gate is discovery-driven, so each one needs a case
proving it fails when it should -- otherwise the gate is a green light with
nothing behind it.

Each case copies the real tree, breaks one thing, and requires the gate to
notice. Breaking nothing must stay clean, or the cases prove only that the gate
fails on everything.

Run: python3 test/docs/consistency_check_test.py
Exit code 0 when every case holds, 1 otherwise.
"""

import os
import shutil
import sys
import tempfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import consistency_check as gate  # noqa: E402

REPO = gate.REPO
CASES = []
problems = []


def case(name):
    def register(fn):
        CASES.append((name, fn))
        return fn
    return register


# What the fixture has to contain: everything the documents point at. Copying
# only docs/ made every link out of the tree look broken, which the cases
# caught -- the fixture is part of what is being tested.
FIXTURE_CONTENTS = ("docs", "AGENTS.md", "CONTRIBUTING.md", "README.md", "gmp-core", "test")


def copy_tree():
    root = tempfile.mkdtemp(prefix="docs-gate-")
    for relative in FIXTURE_CONTENTS:
        source = os.path.join(REPO, relative)
        target = os.path.join(root, relative)
        if os.path.isdir(source):
            shutil.copytree(source, target,
                            ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
        elif os.path.exists(source):
            os.makedirs(os.path.dirname(target), exist_ok=True)
            shutil.copy2(source, target)
    return root


def edit(root, relative, old, new):
    path = os.path.join(root, relative)
    with open(path, encoding="utf-8") as handle:
        body = handle.read()
    if body.count(old) != 1:
        raise AssertionError("fixture edit matched %d times in %s: %r" % (body.count(old), relative, old[:40]))
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(body.replace(old, new))


def checks_that_failed(root):
    return {item.split("]")[0].lstrip("[") for item in gate.run(root)}


def expect_failure(root, check, what):
    failed = checks_that_failed(root)
    if check not in failed:
        problems.append("%s：破坏之后 %r 没有报错（实际报错的是 %s）"
                        % (what, check, "、".join(sorted(failed)) or "无"))


@case("未破坏时必须干净")
def unbroken_is_clean(root):
    found = gate.run(root)
    if found:
        problems.append("未破坏的副本上报了 %d 处：%s" % (len(found), found[:2]))


@case("整个 docs/domain 消失")
def domain_gone(root):
    shutil.rmtree(os.path.join(root, "docs", "domain"))
    failed = checks_that_failed(root)
    for check in ("struck-fact-cited", "numbering", "restated-ban"):
        if check not in failed:
            problems.append("docs/domain 消失后 %r 仍然通过——发现驱动的循环零次执行也算通过" % check)


@case("变更证据目录消失")
def changes_gone(root):
    shutil.rmtree(os.path.join(root, "docs", "history", "changes"))
    expect_failure(root, "record-vs-product", "变更证据目录消失")


@case("README 的文档清单标题被改写")
def classification_heading_renamed(root):
    edit(root, "docs/domain/README.md", "## 文档清单", "## 文档一览")
    expect_failure(root, "classification-grounds", "文档清单标题被改写")


@case("引用了已划线推翻的事实（同文档内）")
def struck_cited_inside(root):
    edit(root, "docs/domain/campaign-user-stories.md",
         "| C6 | 一个投放项能否同时是两类 | **不能**",
         "| C6 | 一个投放项能否同时是两类（见事实 24） | **不能**")
    expect_failure(root, "struck-fact-cited", "同文档内引用被推翻的事实")


@case("引用了另一文档中不存在的事实")
def cross_document_missing_fact(root):
    edit(root, "docs/domain/interaction-invariants.md",
         "受众侧事实 4、5、16、17", "受众侧事实 4、5、16、17、99")
    expect_failure(root, "cross-document-citation", "引用另一文档不存在的事实")


@case("引用了另一文档中已被推翻的事实")
def cross_document_struck_fact(root):
    edit(root, "docs/domain/interaction-invariants.md",
         "| 事实 20（对照组必须在系统里存在） | 承接进不变量 3 |",
         "| 事实 20、24（对照组必须在系统里存在） | 承接进不变量 3 |")
    expect_failure(root, "cross-document-citation", "引用另一文档已被推翻的事实")


@case("来源文档的事实清单标题被改写")
def fact_list_heading_renamed(root):
    edit(root, "docs/domain/campaign-user-stories.md", "## 已确认事实清单", "## 已确认的事实")
    expect_failure(root, "cross-document-citation", "来源文档的事实清单标题被改写")


@case("设计文档缺一个必备章节")
def design_missing_section(root):
    edit(root, "docs/domain/interaction-invariants.md", "## 状态机", "## 状态机变体")
    expect_failure(root, "design-document", "设计文档缺必备章节")


@case("承接表漏登一条来源事实")
def disposition_incomplete(root):
    edit(root, "docs/domain/interaction-invariants.md",
         "| 事实 19（回流延迟小时级够用，平台应在界面上拦住更短的配置） | 承接进不变量 6 |", "")
    expect_failure(root, "design-document", "承接表漏登来源事实")


@case("编号不连续")
def numbering_broken(root):
    edit(root, "docs/domain/campaign-user-stories.md",
         "| 2 | 作为运营，我想在一个活动下面挂**好几个投放项**",
         "| 3 | 作为运营，我想在一个活动下面挂**好几个投放项**")
    expect_failure(root, "numbering", "故事编号不连续")


@case("复述了规则真源的禁令")
def ban_restated(root):
    edit(root, "docs/domain/business-scenarios.md", "## 覆盖矩阵", "## 覆盖矩阵\n\n本节不写实现。\n")
    expect_failure(root, "restated-ban", "复述规则真源的禁令")


@case("同一语义色给了两个取值")
def palette_conflict(root):
    edit(root, "docs/product/design-system.md",
         "| 描边 | `#CFC9BE` | 次级按钮、输入框边框 |",
         "| 描边 | `#CFC9BE` | 次级按钮 |\n| 描边 | `#D0D0D0` | 输入框边框 |")
    expect_failure(root, "palette", "同一角色两个取值")


@case("色板小节标题被改写")
def palette_heading_renamed(root):
    edit(root, "docs/product/design-system.md", "### 色板", "### 颜色表")
    expect_failure(root, "palette", "色板小节标题被改写")


@case("对比度写了一个好看的假值")
def contrast_lies(root):
    edit(root, "docs/product/design-system.md",
         "| 三级文字 | 纸底 | 4.96:1 |", "| 三级文字 | 纸底 | 9.99:1 |")
    expect_failure(root, "contrast", "对比度与实测不符")


@case("把一个不达标的取值放回色板")
def contrast_below_floor(root):
    edit(root, "docs/product/design-system.md",
         "| 三级文字 | `#736C63` |", "| 三级文字 | `#8A837A` |")
    expect_failure(root, "contrast", "取值低于 AA 正文标准")


@case("链接指向不存在的路径")
def dangling_link(root):
    edit(root, "docs/domain/README.md",
         "[`business-scenarios.md`](business-scenarios.md)",
         "[`business-scenarios.md`](business-scenarios-renamed.md)")
    expect_failure(root, "relative-link", "链接指向不存在的路径")


@case("docs 下所有链接消失")
def no_links_at_all(root):
    for base, _, names in os.walk(os.path.join(root, "docs")):
        for name in names:
            if not name.endswith(".md"):
                continue
            path = os.path.join(base, name)
            with open(path, encoding="utf-8") as handle:
                body = handle.read()
            with open(path, "w", encoding="utf-8") as handle:
                handle.write(gate.RELATIVE_LINK.sub("链接已去除", body))
    agents = os.path.join(root, "AGENTS.md")
    with open(agents, encoding="utf-8") as handle:
        body = handle.read()
    with open(agents, "w", encoding="utf-8") as handle:
        handle.write(gate.RELATIVE_LINK.sub("链接已去除", body))
    expect_failure(root, "relative-link", "所有链接被去除")


@case("整个 docs/product 消失")
def product_gone(root):
    shutil.rmtree(os.path.join(root, "docs", "product"))
    expect_failure(root, "palette", "docs/product 消失")


def main():
    for name, fn in CASES:
        root = copy_tree()
        try:
            fn(root)
        except AssertionError as problem:
            problems.append("%s：用例自身出错——%s" % (name, problem))
        finally:
            shutil.rmtree(root, ignore_errors=True)
    if not CASES:
        print("docs gate mutations: 没有任何用例，本身即失败")
        return 1
    if problems:
        print("docs gate mutations: %d 处不通过\n" % len(problems))
        for problem in problems:
            print("  " + problem)
        return 1
    print("docs gate mutations: 全部通过（%d 个变异用例）" % len(CASES))
    return 0


if __name__ == "__main__":
    sys.exit(main())
