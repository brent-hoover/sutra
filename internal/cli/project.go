package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

func projectCmd(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects (1 project = 1 repo)",
	}
	cmd.AddCommand(
		projectCreateCmd(cfg),
		projectListCmd(cfg),
		projectViewCmd(cfg),
		projectUpdateCmd(cfg),
		projectDeleteCmd(cfg),
	)
	return cmd
}

func projectCreateCmd(cfg config.Config) *cobra.Command {
	var name, repo, slug, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).CreateProject(cmd.Context(), name, repo, slug, description)
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			p := res.Project
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", p.ID, cleanLine(p.Slug), cleanLine(p.RepoPath))
			return err
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "project name (required)")
	cmd.Flags().StringVar(&repo, "repo", "", "absolute path to the repository (required)")
	cmd.Flags().StringVar(&slug, "slug", "", "url-safe slug (defaults to a slugified name)")
	cmd.Flags().StringVar(&description, "description", "", "project description")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

func projectListCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).ListProjects(cmd.Context())
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			var b strings.Builder
			for _, p := range res.Projects {
				fmt.Fprintf(&b, "%s\t%s\t%s\n", p.ID, cleanLine(p.Slug), cleanLine(p.Name))
			}
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
}

func projectViewCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "View a project by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).GetProject(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			p := res.Project
			var b strings.Builder
			fmt.Fprintf(&b, "id:        %s\n", p.ID)
			fmt.Fprintf(&b, "name:      %s\n", cleanLine(p.Name))
			fmt.Fprintf(&b, "slug:      %s\n", cleanLine(p.Slug))
			fmt.Fprintf(&b, "repo:      %s\n", cleanLine(p.RepoPath))
			if p.Description != "" {
				fmt.Fprintf(&b, "\n%s\n", clean(p.Description))
			}
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
}

func projectUpdateCmd(cfg config.Config) *cobra.Command {
	var name, slug, repo, description string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a project's name, slug, repo, or description",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fields := map[string]string{}
			for name, key := range map[string]string{
				"name": "name", "slug": "slug", "repo": "repo_path", "description": "description",
			} {
				if cmd.Flags().Changed(name) {
					fields[key] = cmd.Flags().Lookup(name).Value.String()
				}
			}
			res, err := client.New(cfg).UpdateProject(cmd.Context(), args[0], fields)
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			p := res.Project
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", p.ID, cleanLine(p.Slug), cleanLine(p.Name))
			return err
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&slug, "slug", "", "new slug")
	cmd.Flags().StringVar(&repo, "repo", "", "new repository path")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	return cmd
}

func projectDeleteCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a project (detaches its issues, threads, and transcripts)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := client.New(cfg).DeleteProject(cmd.Context(), args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return err
		},
	}
}
