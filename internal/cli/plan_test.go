package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/spf13/cobra"
)

func TestReadSourceFileAndStdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("from stdin"))
	if got, err := readSource(cmd, "-"); err != nil || got != "from stdin" {
		t.Errorf("readSource(-) = %q, %v", got, err)
	}

	path := filepath.Join(t.TempDir(), "steps.json")
	if err := os.WriteFile(path, []byte("from file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readSource(cmd, path); err != nil || got != "from file" {
		t.Errorf("readSource(file) = %q, %v", got, err)
	}
}

func TestPlanBuildRejectsBothStdin(t *testing.T) {
	cmd := planBuildCmd(config.Config{})
	cmd.SetArgs([]string{"--title", "x", "--prose", "-", "--steps", "-"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "stdin") {
		t.Errorf("err = %v, want a stdin-conflict error", err)
	}
}

func TestOutputPlanFormatting(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	res := client.PlanResult{
		Plan: domain.Issue{ID: "plan1", Subject: "Search", Type: domain.TypePlan, Status: domain.StatusOpen, Approval: domain.ApprovalPending},
		Children: []domain.Issue{
			{ID: "c1", Subject: "first"},
			{ID: "c2", Subject: "second"},
		},
	}
	if err := outputPlan(cmd, res); err != nil {
		t.Fatalf("outputPlan: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"plan1", "pending", "c1", "c2", "chain: c1 → c2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
