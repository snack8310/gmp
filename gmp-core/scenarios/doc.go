// Package scenarios runs the business scenarios across all three layers.
//
// The boundaries document puts cross-subproject scenario tests here, and
// leaves the ones touching only a single subproject with that subproject --
// bucketing stability stays with experimentation, the agreement between
// listing and deciding stays with audience. What lands here is what no single
// layer can show on its own.
//
// Every scenario runs over explicit business values and pure state transitions.
// No database, channel, or outside link is constructed here: the point is to
// keep the cross-layer business cases as regression tests for the core model.
//
// ArmCampaign is one campaign holding the arms of one assignment, and it has
// no edges. ReceiptFlow is the other shape: edges on one campaign's flow.
// TwoStep stays two campaigns joined by backflow, which is how a wait is
// expressed. An edge's condition is a receipt outcome; this package does not
// invent a time condition to cover the wait.
package scenarios
