package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

// planCmd groups the plan subcommands: build a plan tree, approve a plan.
func planCmd(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Build and approve plan issue trees",
	}
	cmd.AddCommand(planBuildCmd(cfg), planApproveCmd(cfg))
	return cmd
}

func planBuildCmd(cfg config.Config) *cobra.Command {
	var title, prose, steps, parent, project string
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a plan issue and its tracer children from a steps file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if prose == "-" && steps == "-" {
				return fmt.Errorf("only one of --prose or --steps may read from stdin")
			}
			proseText, err := readSource(cmd, prose)
			if err != nil {
				return fmt.Errorf("read prose: %w", err)
			}
			stepsRaw, err := readSource(cmd, steps)
			if err != nil {
				return fmt.Errorf("read steps: %w", err)
			}
			var stepList []client.PlanStepInput
			if err := json.Unmarshal([]byte(stepsRaw), &stepList); err != nil {
				return fmt.Errorf("parse steps JSON: %w", err)
			}
			res, err := client.New(cfg).BuildPlan(cmd.Context(), title, proseText, stepList, parent, project)
			if err != nil {
				return err
			}
			return outputPlan(cmd, res)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "plan issue subject (required)")
	cmd.Flags().StringVar(&prose, "prose", "", "plan prose: a file path, or - for stdin (required)")
	cmd.Flags().StringVar(&steps, "steps", "", "tracer items as a JSON array: a file path, or - for stdin (required)")
	cmd.Flags().StringVar(&parent, "parent", "", "feature issue id this plan plans (optional)")
	cmd.Flags().StringVar(&project, "project", "", "project id to scope the plan (optional)")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("prose")
	_ = cmd.MarkFlagRequired("steps")
	return cmd
}

func planApproveCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "approve <plan-id>",
		Short: "Approve a plan issue (pending → approved)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).ApprovePlan(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			i := res.Issue
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t[%s/%s/%s]\t%s\n",
				i.ID, i.Type, i.Status, i.Approval, cleanLine(i.Subject))
			return err
		},
	}
}

// readSource returns the contents of spec: stdin when spec is "-", otherwise the
// named file.
func readSource(cmd *cobra.Command, spec string) (string, error) {
	if spec == "-" {
		b, err := io.ReadAll(cmd.InOrStdin())
		return string(b), err
	}
	b, err := os.ReadFile(spec)
	return string(b), err
}

// outputPlan prints the built plan, its children, and the blocking chain (or raw
// JSON with --json).
func outputPlan(cmd *cobra.Command, res client.PlanResult) error {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return printRawJSON(cmd.OutOrStdout(), res.Raw)
	}
	p := res.Plan
	var b strings.Builder
	fmt.Fprintf(&b, "plan %s\t[%s/%s/%s]\t%s\n", p.ID, p.Type, p.Status, p.Approval, cleanLine(p.Subject))
	ids := make([]string, len(res.Children))
	for i, c := range res.Children {
		ids[i] = c.ID
		fmt.Fprintf(&b, "  %d. %s\t%s\n", i+1, c.ID, cleanLine(c.Subject))
	}
	if len(ids) > 1 {
		fmt.Fprintf(&b, "chain: %s\n", strings.Join(ids, " → "))
	}
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}
