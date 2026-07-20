package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSkillHappyPath(t *testing.T) {
	target := t.TempDir()
	path, err := writeSkill(target, "graphify", "SKILL body")
	if err != nil {
		t.Fatalf("writeSkill: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "SKILL body" {
		t.Errorf("content = %q, want %q", got, "SKILL body")
	}
	if want := filepath.Join(target, "graphify", "SKILL.md"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

// A pre-existing directory symlink at <target>/<slug> that escapes the target
// must not be followed: the write is refused and nothing lands outside target.
func TestWriteSkillRejectsEscapingDirSymlink(t *testing.T) {
	target := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(target, "graphify")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, err := writeSkill(target, "graphify", "x"); err == nil {
		t.Fatal("writeSkill through an escaping dir symlink: got nil error, want refusal")
	}
	if _, err := os.Stat(filepath.Join(outside, "SKILL.md")); err == nil {
		t.Error("SKILL.md was written outside the target via the symlink")
	}
}

// A pre-existing SKILL.md symlink that escapes the target must not be followed.
func TestWriteSkillRejectsEscapingFileSymlink(t *testing.T) {
	target := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, "graphify"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "escaped.md"), filepath.Join(target, "graphify", "SKILL.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, err := writeSkill(target, "graphify", "x"); err == nil {
		t.Fatal("writeSkill through an escaping file symlink: got nil error, want refusal")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.md")); err == nil {
		t.Error("content was written outside the target via the file symlink")
	}
}
