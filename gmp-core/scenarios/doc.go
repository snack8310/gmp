// Package scenarios runs the business scenarios across all three layers.
//
// The boundaries document puts cross-subproject scenario tests here, and
// leaves the ones touching only a single subproject with that subproject --
// bucketing stability stays with experimentation, the agreement between
// listing and deciding stays with audience. What lands here is what no single
// layer can show on its own.
//
// Everything runs over the in-memory implementations. No database, no channel,
// no outside link: the point is to get real results out of the scenarios and
// keep them as regression cases for the core model, not to stand a system up.
package scenarios
