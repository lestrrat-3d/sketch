// Package sketchtest provides assertions for tests that use sketch.
//
// Every helper takes testing.TB first, marks itself with Helper, and stops the
// test with Fatalf when its assertion fails. Helpers use only sketch's public
// API and do not redefine the engine's verification or profile verdicts.
// See docs/sketchtest-design.md in the repository for the complete contract.
package sketchtest
