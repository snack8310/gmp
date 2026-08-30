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
DOMAIN = os.path.join(REPO, "docs", "domain")
CHANGES = os.path.join(REPO, "docs", "history", "changes")

failures = []


def fail(check, path, line, msg):
    where = "%s:%s" % (os.path.relpath(path, REPO), line) if line else os.path.relpath(path, REPO)
    failures.append("[%s] %s\n    %s" % (check, where, msg))


def read(path):
    with open(path, encoding="utf-8") as handle:
        return handle.read().splitlines()


def domain_docs():
    for name in sorted(os.listdir(DOMAIN)):
        if name.endswith(".md"):
            yield os.path.join(DOMAIN, name)


def change_docs():
    for name in sorted(os.listdir(CHANGES)):
        if name.endswith(".md"):
            yield os.path.join(CHANGES, name)


# --- check 1 -----------------------------------------------------------------
# A struck-through fact is retracted. Citing its number as grounds resurrects it
# somewhere the strike-through cannot be seen. This is the defect that survived
# every manual pass.

STRUCK_FACT = re.compile(r"^(\d+)\.\s+~~")
CITES_FACT = re.compile(r"事实\s*((?:\d+)(?:\s*[、,，]\s*\d+)*)")


def check_struck_facts_are_not_cited():
    for path in domain_docs():
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
    for path in domain_docs():
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
                missing = sorted(set(expected) - set(seen))
                repeated = sorted({n for n in seen if seen.count(n) > 1})
                fail(
                    "numbering", path, None,
                    "%s 不连续：缺 %s，重复 %s" % (label, missing or "无", repeated or "无"),
                )


# --- check 3 -----------------------------------------------------------------
# README classifies every document and states the grounds for the classification.
# A classification whose stated grounds are false for the document it classifies
# is a label, not a judgement -- the exact move an evaluator caught twice.

# A decision record must actually rest on confirmed decisions: either it marks
# its own items confirmed, or it cites confirmed items elsewhere by number. A
# document doing neither is classified by label rather than by content.
MARKS_CONFIRMED = re.compile(r"`已确认`|\*\*已确认\*\*")
CITES_CONFIRMED = re.compile(r"事实\s*\d+|(?<![A-Za-z])[ATQCB]\d+(?![A-Za-z])")


def check_classification_grounds_hold():
    classified = {}
    for line in section(read(os.path.join(DOMAIN, "README.md")), "文档清单"):
        hit = re.match(r"^\|\s*\[`([^`]+)`\][^|]*\|\s*([^|]+?)\s*\|", line)
        if hit:
            classified[hit.group(1)] = hit.group(2).strip()
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
    for path in domain_docs():
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


# --- check 5 -----------------------------------------------------------------
# A change record describing products that do not exist is not a record of
# facts. Catches the retracted-rule restatement inside the evidence itself.

def check_change_records_match_products():
    classified = {}
    for line in section(read(os.path.join(DOMAIN, "README.md")), "文档清单"):
        hit = re.match(r"^\|\s*\[`([^`]+)`\][^|]*\|\s*([^|]+?)\s*\|", line)
        if hit:
            classified[hit.group(1)] = hit.group(2).strip()
    if not classified:
        return
    kinds = set(classified.values())
    for path in change_docs():
        for number, line in enumerate(read(path), 1):
            if "需求整理文档" in line and "需求整理" not in kinds:
                fail(
                    "record-vs-product", path, number,
                    "描述了「需求整理文档」，但 README 的分类表中没有任何一份"
                    "（现有分类：%s）" % "、".join(sorted(kinds)),
                )


CHECKS = (
    check_struck_facts_are_not_cited,
    check_numbering_is_contiguous,
    check_classification_grounds_hold,
    check_no_restated_bans,
    check_change_records_match_products,
)


def main():
    for check in CHECKS:
        check()
    if failures:
        print("docs consistency: %d 处不通过\n" % len(failures))
        for item in failures:
            print(item)
        return 1
    print("docs consistency: 全部通过（%d 项检查）" % len(CHECKS))
    return 0


if __name__ == "__main__":
    sys.exit(main())
