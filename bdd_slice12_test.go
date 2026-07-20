package main_test

// Step definitions for the @slice12 skill scenarios: skill CRUD and the
// client-side install that writes a skill's content to the local skills
// directory as <slug>/SKILL.md. Steps drive the real CLI against the daemon.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/brent-hoover/sutra/internal/domain"
)

func registerSlice12Steps(sc *godog.ScenarioContext, w *world) {
	var (
		skillName  string
		skillBody  string
		skillID    string
		skillSlug  string
		listOut    string
		updatedOut string
		installDir string
	)

	createSkill := func(name, content string, extra ...string) (domain.Skill, error) {
		args := append([]string{"skill", "create", "--name", name, "--content", content, "--json"}, extra...)
		out, err := w.runCLI(args...)
		if err != nil {
			return domain.Skill{}, err
		}
		var sk domain.Skill
		err = json.Unmarshal([]byte(strings.TrimSpace(out)), &sk)
		return sk, err
	}

	// --- Create a skill ---
	sc.Step(`^a name and SKILL.md content$`, func() error {
		skillName = "Graphify"
		skillBody = "---\nname: graphify\ndescription: turn input into a graph\n---\n\nDo the thing."
		return nil
	})
	sc.Step(`^I create a skill$`, func() error {
		sk, err := createSkill(skillName, skillBody)
		if err != nil {
			return err
		}
		skillID, skillSlug = sk.ID, sk.Slug
		if sk.CreatedAt.IsZero() || sk.UpdatedAt.IsZero() {
			return fmt.Errorf("expected timestamps set")
		}
		return nil
	})
	sc.Step(`^the skill is stored with a generated id, a slug, and timestamps set$`, func() error {
		if skillID == "" {
			return fmt.Errorf("expected a generated id")
		}
		if skillSlug != "graphify" {
			return fmt.Errorf("slug = %q, want %q", skillSlug, "graphify")
		}
		return nil
	})

	// --- Slug uniqueness (reuses the shared "^it is rejected$" step on w.err) ---
	sc.Step(`^a skill already exists with a slug$`, func() error {
		_, err := createSkill("First", "body one", "--slug", "shared-slug")
		return err
	})
	sc.Step(`^I create another skill with the same slug$`, func() error {
		_, w.err = w.runCLI("skill", "create", "--name", "Second", "--content", "body two", "--slug", "shared-slug", "--json")
		return nil
	})

	// --- List, view, update ---
	sc.Step(`^some skills exist$`, func() error {
		if _, err := createSkill("Alpha", "alpha body"); err != nil {
			return err
		}
		sk, err := createSkill("Beta", "beta body")
		if err != nil {
			return err
		}
		skillID = sk.ID
		return nil
	})
	sc.Step(`^I list them, view one, and update its content$`, func() error {
		var err error
		if listOut, err = w.runCLI("skill", "list", "--json"); err != nil {
			return err
		}
		if _, err = w.runCLI("skill", "view", skillID, "--json"); err != nil {
			return err
		}
		updatedOut, err = w.runCLI("skill", "update", skillID, "--content", "beta body v2", "--json")
		return err
	})
	sc.Step(`^the skill changes are reflected$`, func() error {
		var skills []domain.Skill
		if err := json.Unmarshal([]byte(strings.TrimSpace(listOut)), &skills); err != nil {
			return err
		}
		if len(skills) < 2 {
			return fmt.Errorf("expected at least 2 skills, got %d", len(skills))
		}
		var sk domain.Skill
		if err := json.Unmarshal([]byte(strings.TrimSpace(updatedOut)), &sk); err != nil {
			return err
		}
		if sk.Content != "beta body v2" {
			return fmt.Errorf("updated content = %q, want %q", sk.Content, "beta body v2")
		}
		return nil
	})

	// --- Install / delete share "^a skill$" ---
	sc.Step(`^a skill$`, func() error {
		sk, err := createSkill("Installable", "---\nname: installable\n---\n\nSkill body here.")
		if err != nil {
			return err
		}
		skillID, skillSlug, skillBody = sk.ID, sk.Slug, sk.Content
		return nil
	})
	sc.Step(`^I install it to a target directory$`, func() error {
		installDir = filepath.Join(w.dir, "install-target")
		_, err := w.runCLI("skill", "install", skillID, "--dir", installDir)
		return err
	})
	sc.Step(`^its SKILL.md is written under that directory as <slug>/SKILL\.md with the content$`, func() error {
		path := filepath.Join(installDir, skillSlug, "SKILL.md")
		got, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read installed skill: %w", err)
		}
		if string(got) != skillBody {
			return fmt.Errorf("installed content mismatch:\n got: %q\nwant: %q", string(got), skillBody)
		}
		return nil
	})
	sc.Step(`^I delete the skill$`, func() error {
		_, err := w.runCLI("skill", "delete", skillID)
		return err
	})
	sc.Step(`^it is gone$`, func() error {
		if _, err := w.runCLI("skill", "view", skillID, "--json"); err == nil {
			return fmt.Errorf("expected skill to be gone")
		}
		return nil
	})
}
