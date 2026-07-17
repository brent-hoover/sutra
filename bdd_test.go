package main_test

// Runs the godog BDD suite over features/.
//
// TestImplemented runs only scenarios tagged for completed slices and must be
// green — it is what `go test ./...` checks by default. TestBacklog runs the
// full spec suite (pending slices show as undefined/red) and is opt-in via
// SUTRA_BACKLOG=1, so unimplemented future work doesn't fail the default run.
// Extend implementedTags as each slice lands; godog's OR is a comma
// (e.g. "@slice1,@slice2").

import (
	"os"
	"testing"

	"github.com/cucumber/godog"
)

const implementedTags = "@slice1,@slice3,@slice5,@slice6,@slice7"

func TestImplemented(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Tags:     implementedTags,
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("implemented scenarios must pass")
	}
}

func TestBacklog(t *testing.T) {
	if os.Getenv("SUTRA_BACKLOG") == "" {
		t.Skip("set SUTRA_BACKLOG=1 to run the full red backlog of pending slices")
	}
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("backlog has pending scenarios (expected until all slices are implemented)")
	}
}
