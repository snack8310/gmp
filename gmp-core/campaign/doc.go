// Package campaign decides what one person should be shown, and decides only
// that.
//
// A treatment produces a result -- which registered action, with which
// parameters -- and never calls anything outside. Keeping the decision apart
// from the doing is what makes it possible to work out what someone would
// receive without sending it, which matters because a send cannot be taken
// back. It is nearly free to arrange now and expensive to retrofit later.
//
// This layer sits above audience and imports nothing else. A rollout is
// expressed as an audience definition -- the shares taken in so far -- because
// a share is an ordinary condition about a person, and asking that way keeps
// this package from reaching past the layer below it.
package campaign
