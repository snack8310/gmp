// Package experimentation answers exactly one question: which share does this
// assignment key fall into.
//
// It is the bottom of the dependency layering inside gmp-core and imports
// nothing from audience, campaign or scenarios. That zero-dependency property
// is what lets the package be lifted out whole later, and it is enforced by a
// check rather than by discipline -- see internal/arch.
//
// The package has no notion of people, segments or time. It takes an opaque
// key and a definition, and it returns a bucket number. Callers decide what
// the key stands for.
package experimentation
