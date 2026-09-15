package audience

// ProvablyExclusive reports whether two definitions can never hold for the
// same person -- with a proof, not a guess.
//
// The only proof this platform has is sharing one assignment: a key lands in
// exactly one share of a given carve-up, so two definitions demanding
// different shares of the same revision cannot both hold. Everything else --
// attributes that look disjoint, windows that seem not to overlap -- is
// refused, because "looks exclusive" is how two treatments end up reaching the
// same person with nothing failing.
//
// The answer is therefore one-directional: true means proven, false means not
// proven, never "proven not".
func ProvablyExclusive(a, b Definition) bool {
	if a.isZero() || b.isZero() {
		return false
	}
	for _, left := range a.conditions {
		leftShare, ok := left.(shareCondition)
		if !ok {
			continue
		}
		for _, right := range b.conditions {
			rightShare, ok := right.(shareCondition)
			if !ok {
				continue
			}
			if !leftShare.assignment.SameRevisionAs(rightShare.assignment) {
				continue
			}
			if leftShare.share != rightShare.share {
				return true
			}
		}
	}
	return false
}

// Subsumes reports whether everyone matching later also matches earlier --
// with a proof, not a guess.
//
// The proof available here is structural: both draw on the same source, and
// earlier demands nothing that later does not also demand. A definition asking
// for strictly more than another cannot reach anyone the other misses.
//
// Like ProvablyExclusive this is one-directional. False means not proven.
// Deciding it in general would take comparing what conditions mean, not what
// they are, and answering that with a guess is how a row nobody can reach ends
// up looking fine.
func Subsumes(earlier, later Definition) bool {
	if earlier.isZero() || later.isZero() {
		return false
	}
	if earlier.source != later.source {
		return false
	}
	for _, required := range earlier.conditions {
		found := false
		for _, candidate := range later.conditions {
			if required.equals(candidate) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
