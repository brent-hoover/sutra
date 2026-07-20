package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

func skillCmd(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage reusable agent skills and install them locally",
	}
	cmd.AddCommand(
		skillCreateCmd(cfg),
		skillListCmd(cfg),
		skillViewCmd(cfg),
		skillUpdateCmd(cfg),
		skillDeleteCmd(cfg),
		skillInstallCmd(cfg),
	)
	return cmd
}

func skillCreateCmd(cfg config.Config) *cobra.Command {
	var name, slug, description, content, contentFile string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a skill",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveContent(content, contentFile)
			if err != nil {
				return err
			}
			res, err := client.New(cfg).CreateSkill(cmd.Context(), name, slug, description, body)
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			sk := res.Skill
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", sk.ID, cleanLine(sk.Slug), cleanLine(sk.Name))
			return err
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "skill name (required)")
	cmd.Flags().StringVar(&slug, "slug", "", "url-safe slug (defaults to a slugified name)")
	cmd.Flags().StringVar(&description, "description", "", "one-line description")
	cmd.Flags().StringVar(&content, "content", "", "SKILL.md content")
	cmd.Flags().StringVar(&contentFile, "content-file", "", "read SKILL.md content from a file ('-' for stdin)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// resolveContent returns the skill content from --content, or read from
// --content-file (or stdin when it is "-"). Exactly one source is expected.
func resolveContent(content, contentFile string) (string, error) {
	if contentFile == "" {
		return content, nil
	}
	if content != "" {
		return "", fmt.Errorf("use only one of --content or --content-file")
	}
	if contentFile == "-" {
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	b, err := os.ReadFile(contentFile)
	return string(b), err
}

func skillListCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).ListSkills(cmd.Context())
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			var b strings.Builder
			for _, sk := range res.Skills {
				fmt.Fprintf(&b, "%s\t%s\t%s\n", sk.ID, cleanLine(sk.Slug), cleanLine(sk.Description))
			}
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
}

func skillViewCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "View a skill (metadata and SKILL.md content)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).GetSkill(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			sk := res.Skill
			var b strings.Builder
			fmt.Fprintf(&b, "id:    %s\n", sk.ID)
			fmt.Fprintf(&b, "name:  %s\n", cleanLine(sk.Name))
			fmt.Fprintf(&b, "slug:  %s\n", cleanLine(sk.Slug))
			if sk.Description != "" {
				fmt.Fprintf(&b, "desc:  %s\n", cleanLine(sk.Description))
			}
			fmt.Fprintf(&b, "\n%s\n", clean(sk.Content))
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
}

func skillUpdateCmd(cfg config.Config) *cobra.Command {
	var name, slug, description, content, contentFile string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a skill's name, slug, description, or content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fields := map[string]string{}
			for _, name := range []string{"name", "slug", "description"} {
				if cmd.Flags().Changed(name) {
					fields[name] = cmd.Flags().Lookup(name).Value.String()
				}
			}
			if cmd.Flags().Changed("content") || cmd.Flags().Changed("content-file") {
				body, err := resolveContent(content, contentFile)
				if err != nil {
					return err
				}
				fields["content"] = body
			}
			res, err := client.New(cfg).UpdateSkill(cmd.Context(), args[0], fields)
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			sk := res.Skill
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", sk.ID, cleanLine(sk.Slug), cleanLine(sk.Name))
			return err
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&slug, "slug", "", "new slug")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().StringVar(&content, "content", "", "new SKILL.md content")
	cmd.Flags().StringVar(&contentFile, "content-file", "", "read new content from a file ('-' for stdin)")
	return cmd
}

func skillDeleteCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := client.New(cfg).DeleteSkill(cmd.Context(), args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return err
		},
	}
}

func skillInstallCmd(cfg config.Config) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "install <id>",
		Short: "Fetch a skill and write it to the local skills directory as <slug>/SKILL.md",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).GetSkill(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			sk := res.Skill

			target := dir
			if target == "" {
				target = cfg.SkillsDir
			}
			if target == "" {
				target = config.DefaultSkillsDir()
			}
			// The slug must be a single, safe path element (Slugify guarantees this
			// for created skills; guard defensively against a crafted stored slug).
			if sk.Slug == "" || sk.Slug == "." || sk.Slug == ".." || strings.ContainsAny(sk.Slug, `/\`) {
				return fmt.Errorf("refusing to install skill with unsafe slug %q", sk.Slug)
			}
			path, err := writeSkill(target, sk.Slug, sk.Content)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "installed %s to %s\n", sk.Slug, path)
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "target skills directory (default: config skills_dir, else ~/.claude/skills)")
	return cmd
}

// writeSkill writes content to <target>/<slug>/SKILL.md, creating target if
// needed. It writes through a pinned os.Root at target, so a pre-existing
// symlink at <slug> or SKILL.md that escapes target is refused rather than
// followed (no writes outside the configured skills directory). Returns the
// written path.
func writeSkill(target, slug, content string) (string, error) {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		return "", err
	}
	defer root.Close()

	if err := root.Mkdir(slug, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	rel := filepath.Join(slug, "SKILL.md")
	f, err := root.OpenFile(rel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	return filepath.Join(target, rel), nil
}
