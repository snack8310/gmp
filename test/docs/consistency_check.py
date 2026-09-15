#!/usr/bin/env python3
"""Turns four failure modes of the docs review loop into a gate.

Each check exists because manual review missed the same class of defect on
four consecutive evaluator rounds. See docs/history/memory/ for the rules
these enforce; per AGENTS.md the point is to stop relying on discipline for
something a script can decide.

Run: python3 test/docs/consistency_check.py
Exit code 0 when clean, 1 when any check fails.
"""

import os
import re
import sys

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# The tree under inspection. It is a variable rather than a constant so that
# the checks can be pointed at a fixture: a gate nobody can aim somewhere else
# is a gate nobody can show the teeth of.
ROOT = REPO
DOMAIN = os.path.join(ROOT, "docs", "domain")
CHANGES = os.path.join(ROOT, "docs", "history", "changes")
PRODUCT = os.path.join(ROOT, "docs", "product")

failures = []


def use_root(root):
    """Point every check at the given tree."""
    global ROOT, DOMAIN, CHANGES, PRODUCT
    ROOT = root
    DOMAIN = os.path.join(root, "docs", "domain")
    CHANGES = os.path.join(root, "docs", "history", "changes")
    PRODUCT = os.path.join(root, "docs", "product")


def fail(check, path, line, msg):
    where = "%s:%s" % (os.path.relpath(path, ROOT), line) if line else os.path.relpath(path, ROOT)
    failures.append("[%s] %s\n    %s" % (check, where, msg))


def missing(check, what):
    """Record that a check had nothing to look at.

    Every check below finds its own subjects, which means every one of them can
    pass by finding none: rename a directory or a heading and the loop body
    never runs while the exit code stays zero. So each traversal states what it
    expects to find, and finding nothing is itself a failure.
    """
    failures.append("[%s] %s\n    %s" % (check, "遍历基准为空", what))


def read(path):
    if not os.path.exists(path):
        return []
    with open(path, encoding="utf-8") as handle:
        return handle.read().splitlines()


def markdown_in(directory):
    if not os.path.isdir(directory):
        return
    for name in sorted(os.listdir(directory)):
        if name.endswith(".md"):
            yield os.path.join(directory, name)


def domain_docs():
    return markdown_in(DOMAIN)


def change_docs():
    return markdown_in(CHANGES)


def product_docs():
    return markdown_in(PRODUCT)


# --- check 1 -----------------------------------------------------------------
# A struck-through fact is retracted. Citing its number as grounds resurrects it
# somewhere the strike-through cannot be seen. This is the defect that survived
# every manual pass.

STRUCK_FACT = re.compile(r"^(\d+)\.\s+~~")
CITES_FACT = re.compile(r"事实\s*((?:\d+)(?:\s*[、,，]\s*\d+)*)")


def check_struck_facts_are_not_cited():
    examined = 0
    for path in domain_docs():
        examined += 1
        lines = read(path)
        struck = set()
        for line in lines:
            hit = STRUCK_FACT.match(line)
            if hit:
                struck.add(hit.group(1))
        if not struck:
            continue
        # A line that is itself about the retraction cites the number to point
        # at it, not to lean on it.
        discusses_retraction = re.compile(r"推翻|作废|曾写成|已改|已修正")
        for number, line in enumerate(lines, 1):
            if STRUCK_FACT.match(line) or discusses_retraction.search(line):
                continue
            for group in CITES_FACT.findall(line):
                for cited in re.split(r"[、,，]\s*", group):
                    cited = cited.strip()
                    if cited in struck:
                        fail(
                            "struck-fact-cited", path, number,
                            "引用了已划线推翻的事实 %s 作为依据；划线只在事实清单可见，此处看不到" % cited,
                        )
    if examined == 0:
        missing("struck-fact-cited", "docs/domain/ 下没有找到任何 Markdown 文档")


# --- check 2 -----------------------------------------------------------------
# Numbering that skips or repeats means an entry was added or removed without
# the cross-references being revisited.

NUMBERED_ROW = re.compile(r"^\|\s*(\d+)\s*\|")
NUMBERED_FACT = re.compile(r"^(\d+)\.\s")


def section(lines, heading):
    """Lines under the given heading, up to the next heading of any level."""
    out, inside = [], False
    for line in lines:
        if line.startswith("#"):
            if inside:
                break
            inside = heading in line
            continue
        if inside:
            out.append(line)
    return out


def check_numbering_is_contiguous():
    # Two traversal bases, counted apart. Summing them would let one disappear
    # while the other kept the total above zero -- the very thing a lower bound
    # exists to catch.
    seen_domain, seen_product = 0, 0
    for path in list(domain_docs()) + list(product_docs()):
        if os.path.dirname(path) == DOMAIN:
            seen_domain += 1
        else:
            seen_product += 1
        lines = read(path)
        for label, pattern, scope in (
            ("故事编号", NUMBERED_ROW, lines),
            ("事实编号", NUMBERED_FACT, section(lines, "已确认事实清单")),
        ):
            seen = [int(hit.group(1)) for hit in (pattern.match(l) for l in scope) if hit]
            if not seen:
                continue
            expected = list(range(1, len(seen) + 1))
            if seen != expected:
                absent = sorted(set(expected) - set(seen))
                repeated = sorted({n for n in seen if seen.count(n) > 1})
                fail(
                    "numbering", path, None,
                    "%s 不连续：缺 %s，重复 %s" % (label, absent or "无", repeated or "无"),
                )
    if seen_domain == 0:
        missing("numbering", "docs/domain/ 下没有找到任何 Markdown 文档")
    if seen_product == 0:
        missing("numbering", "docs/product/ 下没有找到任何 Markdown 文档")


# --- check 3 -----------------------------------------------------------------
# README classifies every document and states the grounds for the classification.
# A classification whose stated grounds are false for the document it classifies
# is a label, not a judgement -- the exact move an evaluator caught twice.

# A decision record must actually rest on confirmed decisions: either it marks
# its own items confirmed, or it cites confirmed items elsewhere by number. A
# document doing neither is classified by label rather than by content.
MARKS_CONFIRMED = re.compile(r"`已确认`|\*\*已确认\*\*")
CITES_CONFIRMED = re.compile(r"事实\s*\d+|(?<![A-Za-z])[ATQCB]\d+(?![A-Za-z])")


def classified_docs():
    """Documents README classifies, by name."""
    out = {}
    for line in section(read(os.path.join(DOMAIN, "README.md")), "文档清单"):
        hit = re.match(r"^\|\s*\[`([^`]+)`\][^|]*\|\s*([^|]+?)\s*\|", line)
        if hit:
            out[hit.group(1)] = hit.group(2).strip()
    return out


def check_classification_grounds_hold():
    classified = classified_docs()
    if not classified:
        missing("classification-grounds", "README 的「文档清单」没有解析出任何文档；标题被改写或表格被删除时，本检查会零条目通过")
        return
    for name, kind in sorted(classified.items()):
        if kind != "决策记录":
            continue
        path = os.path.join(DOMAIN, name)
        if not os.path.exists(path):
            fail("classification-grounds", os.path.join(DOMAIN, "README.md"), None,
                 "分类表列出的 %s 不存在" % name)
            continue
        body = "\n".join(read(path))
        marks = len(MARKS_CONFIRMED.findall(body))
        cites = len(CITES_CONFIRMED.findall(body))
        if marks == 0 and cites == 0:
            fail(
                "classification-grounds", path, None,
                "归类为「决策记录」，但既不标记自己的已确认项（%d 处），"
                "也不引用别处的已确认条目（%d 处）——这是靠标签归类，不是靠内容"
                % (marks, cites),
            )


# --- check 4 -----------------------------------------------------------------
# One rule source. A ban repeated at the top of a document outlives its
# retraction in the rule source, and then two rules disagree about the same
# sentence.

RESTATED_BANS = [
    "不写实现",
    "不写「系统怎么做」",
    "不写怎么做",
    "不写任何设计",
]


def check_no_restated_bans():
    seen_domain, seen_product = 0, 0
    for path in list(domain_docs()) + list(product_docs()):
        if os.path.dirname(path) == DOMAIN:
            seen_domain += 1
        else:
            seen_product += 1
        if os.path.basename(path) == "README.md":
            continue
        for number, line in enumerate(read(path), 1):
            for ban in RESTATED_BANS:
                if ban in line:
                    fail(
                        "restated-ban", path, number,
                        "复述了规则真源的禁令「%s」。README 是唯一规则出处，"
                        "此处复述会在规则修订后独立存活" % ban,
                    )
    if seen_domain == 0:
        missing("restated-ban", "docs/domain/ 下没有找到任何 Markdown 文档")
    if seen_product == 0:
        missing("restated-ban", "docs/product/ 下没有找到任何 Markdown 文档")


# --- check 5 -----------------------------------------------------------------
# A change record describing products that do not exist is not a record of
# facts. Catches the retracted-rule restatement inside the evidence itself.

def check_change_records_match_products():
    classified = classified_docs()
    if not classified:
        missing("record-vs-product", "README 的「文档清单」没有解析出任何文档；标题被改写或表格被删除时，本检查会零条目通过")
        return
    kinds = set(classified.values())
    examined = 0
    for path in change_docs():
        examined += 1
        for number, line in enumerate(read(path), 1):
            if "需求整理文档" in line and "需求整理" not in kinds:
                fail(
                    "record-vs-product", path, number,
                    "描述了「需求整理文档」，但 README 的分类表中没有任何一份"
                    "（现有分类：%s）" % "、".join(sorted(kinds)),
                )
    if examined == 0:
        missing("record-vs-product", "docs/history/changes/ 下没有找到任何变更证据")



# --- check 6 -----------------------------------------------------------------
# Check 1 only ever looked inside one file: it found that file's struck facts
# and then that file's citations. A document citing another document's facts
# was therefore unchecked, which is where the design document sits -- every
# fact it leans on belongs to somewhere else. A fact struck later would leave
# it citing a retraction, and the strike-through is only visible in the list.

FACT_OWNERS = {
    "受众侧": "audience-user-stories.md",
    "活动侧": "campaign-user-stories.md",
    "活动文档": "campaign-user-stories.md",
    "活动": "campaign-user-stories.md",
}
# A citation with no qualifier means this document's own facts where it keeps a
# list, and the campaign document otherwise -- the convention every document
# here already follows, spelled out so the check can apply it rather than guess.
UNQUALIFIED_OWNER = "campaign-user-stories.md"
QUALIFIED_CITATION = re.compile(r"(受众侧|活动侧|活动文档|活动)事实\s*((?:\d+)(?:\s*[、,，]\s*\d+)*)")
DISCUSSES_RETRACTION = re.compile(r"推翻|作废|划线|曾写成|已改|已修正")


def fact_list_of(path):
    """Fact numbers in this document, and which of them are struck."""
    lines = read(path)
    inside = False
    present, struck = set(), set()
    for line in lines:
        if line.startswith("#"):
            if inside:
                break
            inside = "已确认事实清单" in line
            continue
        if not inside:
            continue
        hit = re.match(r"^(\d+)\.\s", line)
        if hit:
            present.add(hit.group(1))
            if re.match(r"^\d+\.\s+~~", line) or re.match(r"^\d+\.\s+\*\*?~~", line):
                struck.add(hit.group(1))
    return present, struck


def check_cross_document_fact_citations():
    owners = {}
    for label, name in FACT_OWNERS.items():
        path = os.path.join(DOMAIN, name)
        if not os.path.exists(path):
            fail("cross-document-citation", os.path.join(DOMAIN, name), None,
                 "被引用的来源文档不存在，凡是引用它的地方都无从核对")
            continue
        present, struck = fact_list_of(path)
        if not present:
            missing("cross-document-citation",
                    "%s 的「已确认事实清单」解析不出任何条目；标题被改写时本检查会零条目通过" % name)
            continue
        owners[label] = (name, present, struck)
    if not owners:
        return

    by_name = {name: (name, present, struck) for name, present, struck in owners.values()}

    examined = 0
    for path in domain_docs():
        base = os.path.basename(path)
        # A document keeping its own list is check 1's business; here we resolve
        # what it says about somewhere else.
        own_list, _ = fact_list_of(path)
        default = None if own_list else by_name.get(UNQUALIFIED_OWNER)
        for number, line in enumerate(read(path), 1):
            if DISCUSSES_RETRACTION.search(line):
                continue
            targets = []
            for label, group in QUALIFIED_CITATION.findall(line):
                owner = owners.get(label)
                if owner is not None:
                    targets.append((owner, group))
            if default is not None:
                stripped = QUALIFIED_CITATION.sub("", line)
                for group in CITES_FACT.findall(stripped):
                    targets.append((default, group))
            for (name, present, struck), group in targets:
                if name == base:
                    continue
                for cited in re.split(r"[、,，]\s*", group):
                    cited = cited.strip()
                    examined += 1
                    if cited not in present:
                        fail("cross-document-citation", path, number,
                             "引用了 %s 的事实 %s，而该文档没有这一条" % (name, cited))
                    elif cited in struck:
                        fail("cross-document-citation", path, number,
                             "引用了 %s 中已划线推翻的事实 %s 作为依据" % (name, cited))
    if examined == 0:
        missing("cross-document-citation",
                "docs/domain/ 下没有任何带限定词的跨文档事实引用；引用形态一改本检查即零条目通过")


# --- check 7 -----------------------------------------------------------------
# A design document is assembled from other documents, and README requires the
# assembly to be shown as a table rather than asserted. A table that quietly
# stops short of its own stated basis reads exactly like a complete one -- that
# is how four entries went unlisted and survived a by-hand review.

REQUIRED_DESIGN_SECTIONS = (
    "状态", "名词表", "概念与关系", "不变量",
    "状态机", "边界与非目标", "决策记录", "待澄清问题清单",
)


def check_design_documents_are_complete():
    designs = [name for name, kind in classified_docs().items() if kind == "设计文档"]
    if not designs:
        missing("design-document",
                "README 的「文档清单」里没有任何设计文档；分类被改写时本检查会零条目通过")
        return
    for name in designs:
        path = os.path.join(DOMAIN, name)
        if not os.path.exists(path):
            fail("design-document", path, None, "分类表列出的设计文档不存在")
            continue
        headings = "\n".join(line for line in read(path) if line.startswith("#"))
        for required in REQUIRED_DESIGN_SECTIONS:
            if not re.search(r"^#+\s*%s\s*$" % re.escape(required), headings, re.M):
                fail("design-document", path, None,
                     "缺少 README 要求的章节「%s」" % required)

        # Its disposition table must reach the end of the basis it declares.
        body = "\n".join(read(path))
        if "来源侧承接表" not in body:
            fail("design-document", path, None,
                 "没有「来源侧承接表」；README 的编制纪律要求承接情况落成产物形态，不得只写已逐项核对")
            continue
        table = body[body.index("来源侧承接表"):]
        # Group the qualifiers by the file they point at: several of them name
        # the same document, and the basis is per document, not per spelling.
        for owner in sorted(set(FACT_OWNERS.values())):
            owner_path = os.path.join(DOMAIN, owner)
            if not os.path.exists(owner_path):
                continue
            present, _ = fact_list_of(owner_path)
            if not present:
                continue
            labels = {label for label, name in FACT_OWNERS.items() if name == owner}
            cited = set()
            for found_label, group in QUALIFIED_CITATION.findall(table):
                if found_label not in labels:
                    continue
                cited |= {n.strip() for n in re.split(r"[、,，]\s*", group)}
            if owner == UNQUALIFIED_OWNER:
                stripped = QUALIFIED_CITATION.sub("", table)
                for group in CITES_FACT.findall(stripped):
                    cited |= {n.strip() for n in re.split(r"[、,，]\s*", group)}
            absent = sorted(present - cited, key=int)
            if absent:
                fail("design-document", path, None,
                     "来源侧承接表声明以 %s 的事实清单为遍历基准，但未登记去向：%s"
                     % (owner, "、".join(absent)))



# --- check 8 -----------------------------------------------------------------
# The same semantic colour carrying two values never fails; two parts of the
# interface just drift apart. And a stated contrast ratio is a measurement, so
# it can be recomputed -- which matters because the one value that turned out
# to be below the floor looked entirely fine to the eye, including the eye of
# whoever wrote it down.

PALETTE_ROW = re.compile(r"^\|\s*([^|]+?)\s*\|\s*`(#[0-9A-Fa-f]{6})`\s*\|")
CONTRAST_ROW = re.compile(r"^\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|\s*([0-9.]+):1\s*\|")
AA_BODY_TEXT = 4.5


def relative_luminance(hex_colour):
    channels = []
    for offset in (1, 3, 5):
        value = int(hex_colour[offset:offset + 2], 16) / 255
        channels.append(value / 12.92 if value <= 0.03928 else ((value + 0.055) / 1.055) ** 2.4)
    red, green, blue = channels
    return 0.2126 * red + 0.7152 * green + 0.0722 * blue


def contrast_ratio(first, second):
    a, b = relative_luminance(first), relative_luminance(second)
    high, low = max(a, b), min(a, b)
    return (high + 0.05) / (low + 0.05)


def palette_of(path):
    """Role to colour, and the roles that were given more than one."""
    palette, conflicting = {}, {}
    for line in section(read(path), "色板"):
        hit = PALETTE_ROW.match(line)
        if not hit:
            continue
        role, colour = hit.group(1).strip(), hit.group(2).upper()
        if role in ("角色",) or set(role) <= set("-| "):
            continue
        if role in palette and palette[role] != colour:
            conflicting.setdefault(role, {palette[role]}).add(colour)
        palette[role] = colour
    return palette, conflicting


def check_product_palette_is_single_valued():
    docs = list(product_docs())
    if not docs:
        missing("palette", "docs/product/ 下没有找到任何 Markdown 文档")
        return
    found = 0
    for path in docs:
        palette, conflicting = palette_of(path)
        if not palette:
            continue
        found += 1
        for role, colours in sorted(conflicting.items()):
            fail("palette", path, None,
                 "角色「%s」有多个取值：%s。同一语义色分叉不会报错，只会让两处界面慢慢长得不一样"
                 % (role, "、".join(sorted(colours))))
    if found == 0:
        missing("palette", "docs/product/ 下没有任何「色板」小节；标题被改写时本检查会零条目通过")


def check_product_contrast_is_real():
    docs = list(product_docs())
    if not docs:
        return
    examined = 0
    for path in docs:
        palette, _ = palette_of(path)
        if not palette:
            continue
        for line in section(read(path), "对比度"):
            hit = CONTRAST_ROW.match(line)
            if not hit:
                continue
            foreground, background, stated = hit.group(1).strip(), hit.group(2).strip(), hit.group(3)
            if foreground in ("前景",) or set(foreground) <= set("-| "):
                continue
            if foreground not in palette or background not in palette:
                fail("contrast", path, None,
                     "对比度一行引用了色板里没有的角色：%s on %s" % (foreground, background))
                continue
            examined += 1
            actual = contrast_ratio(palette[foreground], palette[background])
            if abs(actual - float(stated)) > 0.05:
                fail("contrast", path, None,
                     "%s on %s 写的是 %s:1，实测 %.2f:1" % (foreground, background, stated, actual))
            if actual < AA_BODY_TEXT:
                fail("contrast", path, None,
                     "%s on %s 实测 %.2f:1，低于 AA 正文的 %.1f:1" % (foreground, background, actual, AA_BODY_TEXT))
    if examined == 0:
        missing("contrast", "docs/product/ 下没有任何可重算的对比度行；标题被改写时本检查会零条目通过")



# --- check 10 ----------------------------------------------------------------
# A link into the repository that does not resolve is a citation to something
# nobody can fetch. It has been checked by hand every round, which is exactly
# the kind of thing that holds until the round someone is in a hurry.

RELATIVE_LINK = re.compile(r"\[[^\]]*\]\(([^)]+)\)")


def documented_files():
    for base in (DOMAIN, PRODUCT, CHANGES,
                 os.path.join(ROOT, "docs", "history", "memory"),
                 os.path.join(ROOT, "docs", "ai-loop")):
        for path in markdown_in(base):
            yield path
    agents = os.path.join(ROOT, "AGENTS.md")
    if os.path.exists(agents):
        yield agents


def check_relative_links_resolve():
    examined = 0
    for path in documented_files():
        base = os.path.dirname(path)
        for number, line in enumerate(read(path), 1):
            for target in RELATIVE_LINK.findall(line):
                if target.startswith(("http://", "https://", "#", "mailto:")):
                    continue
                target = target.split("#", 1)[0]
                if not target:
                    continue
                examined += 1
                if not os.path.exists(os.path.normpath(os.path.join(base, target))):
                    fail("relative-link", path, number,
                         "链接指向仓库内不存在的路径：%s" % target)
    if examined == 0:
        missing("relative-link", "docs/ 与 AGENTS.md 中没有解析出任何仓库内链接")


CHECKS = (
    check_struck_facts_are_not_cited,
    check_numbering_is_contiguous,
    check_classification_grounds_hold,
    check_no_restated_bans,
    check_change_records_match_products,
    check_cross_document_fact_citations,
    check_design_documents_are_complete,
    check_product_palette_is_single_valued,
    check_product_contrast_is_real,
    check_relative_links_resolve,
)


def run(root):
    """Run every check against the given tree and return what failed."""
    global failures
    use_root(root)
    failures = []
    for check in CHECKS:
        check()
    found = list(failures)
    use_root(REPO)
    failures = []
    return found


def main():
    found = run(REPO)
    if found:
        print("docs consistency: %d 处不通过\n" % len(found))
        for item in found:
            print(item)
        return 1
    print("docs consistency: 全部通过（%d 项检查）" % len(CHECKS))
    return 0


if __name__ == "__main__":
    sys.exit(main())
