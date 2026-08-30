#!/bin/sh
# Regression tests for .githooks/pre-push.
#
# The hook is a protection script whose failure mode is silence: a broken
# version still exits 0 and prints nothing, so it looks healthy while
# protecting nothing. These cases therefore assert exit codes for both the
# reject and the allow paths, and run under every shell available, because
# the defect that motivated this suite (see
# docs/history/memory/shell-script-portability.md) only reproduced under zsh.
#
# Usage: sh test/githooks/pre-push_test.sh
# Exits non-zero if any assertion fails.

set -u

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
hook="$repo_root/.githooks/pre-push"

zero=0000000000000000000000000000000000000000
sha=1111111111111111111111111111111111111111

failures=0
checks=0
skipped_shells=""
ran_shells=""

record() {
	checks=$((checks + 1))
	if [ "$1" -ne "$2" ]; then
		failures=$((failures + 1))
		echo "  FAIL  [$3] $4: expected exit $2, got $1"
	fi
}

# assert <shell> <expected exit> <name> [push line...]
#
# Each push line is fed to the hook terminated by a newline, the way git
# writes them: <local ref> <local sha> <remote ref> <remote sha>.
# With no push line the hook receives empty stdin.
assert() {
	a_shell=$1
	a_want=$2
	a_name=$3
	shift 3

	if [ "$#" -eq 0 ]; then
		printf '' | "$a_shell" "$hook" >/dev/null 2>&1
	else
		for a_line in "$@"; do printf '%s\n' "$a_line"; done |
			"$a_shell" "$hook" >/dev/null 2>&1
	fi
	record "$?" "$a_want" "$a_shell" "$a_name"
}

# assert_raw <shell> <expected exit> <name> <verbatim stdin>
#
# Feeds stdin exactly as given, with no newline appended. Used for the
# no-trailing-newline case, which assert() cannot express.
assert_raw() {
	printf '%s' "$4" | "$1" "$hook" >/dev/null 2>&1
	record "$?" "$2" "$1" "$3"
}

run_suite() {
	s=$1

	# --- reject: protected branches ---
	assert "$s" 1 "push to main" \
		"refs/heads/main $sha refs/heads/main $zero"
	assert "$s" 1 "push to master" \
		"refs/heads/master $sha refs/heads/master $zero"
	assert "$s" 1 "several refs, one of them main" \
		"refs/heads/feat/a $sha refs/heads/feat/a $zero" \
		"refs/heads/main $sha refs/heads/main $zero"
	assert "$s" 1 "main preceded by a blank line" \
		"" \
		"refs/heads/main $sha refs/heads/main $zero"

	# Regression: `read` returns non-zero at EOF when the last line carries no
	# trailing newline, which dropped that line and let a push to main through.
	assert_raw "$s" 1 "push to main, last line without a newline" \
		"refs/heads/main $sha refs/heads/main $zero"
	assert_raw "$s" 0 "push to a feature branch, last line without a newline" \
		"refs/heads/feat/a $sha refs/heads/feat/a $zero"

	# --- allow: everything else ---
	assert "$s" 0 "push to a feature branch" \
		"refs/heads/chore/3-x $sha refs/heads/chore/3-x $zero"
	assert "$s" 0 "delete a remote branch (all-zero local sha)" \
		"(delete) $zero refs/heads/old $sha"
	assert "$s" 0 "push a tag" \
		"refs/tags/v1.0.0 $sha refs/tags/v1.0.0 $zero"
	assert "$s" 0 "empty stdin"
	assert "$s" 0 "malformed line with missing fields" \
		"refs/heads/main $sha"

	# Regression: the branch name must be matched literally -- not as a
	# substring, and not as a glob pattern.
	for name in feat/maintain-x domain-fix main-backup mainx Main foo/main '*' '?ain' 'ma*n'; do
		assert "$s" 0 "branch '$name' is not protected" \
			"refs/heads/$name $sha refs/heads/$name $zero"
	done
}

echo "Testing $hook"

[ -f "$hook" ] || { echo "FAIL  hook not found: $hook"; exit 1; }
[ -x "$hook" ] || { echo "FAIL  hook is not executable: $hook"; exit 1; }

# Every shell that is present must pass. Missing ones are reported rather
# than silently reducing coverage.
for candidate in sh bash dash zsh ksh; do
	if shell_path=$(command -v "$candidate" 2>/dev/null); then
		ran_shells="$ran_shells $candidate"
		run_suite "$shell_path"
	else
		skipped_shells="$skipped_shells $candidate"
	fi
done

echo "Shells run:     ${ran_shells:-none}"
[ -n "$skipped_shells" ] && echo "Shells skipped: $skipped_shells (not installed)"

# A run that exercised no interpreter proves nothing; fail rather than
# reporting green.
if [ -z "$ran_shells" ]; then
	echo "FAIL  no shell interpreter was available"
	exit 1
fi

echo "Assertions:     $checks"

if [ "$failures" -ne 0 ]; then
	echo "FAILED: $failures of $checks"
	exit 1
fi

echo "OK"
