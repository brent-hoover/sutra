package main_test

// Runs the godog BDD suite over features/. Scenarios have no step
// definitions yet, so Strict mode reports undefined steps as failures — the
// intended red state. Implement steps slice by slice per docs/PLAN.md.

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}
