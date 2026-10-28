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
//
// ArmCampaign is one campaign holding the arms of one assignment, and it has
// no edges. ReceiptFlow is the other shape: edges on one campaign's flow.
// TwoStep stays two campaigns joined by backflow, which is how a wait is
// expressed. An edge's condition is a receipt outcome; this package does not
// invent a time condition to cover the wait.
package scenarios
