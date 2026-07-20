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

// A pre-existing SKILL.md symlink that escapes the target must not be written
// through: the atomic rename replaces the symlink with a real file inside the
// target, and nothing lands at the symlink's escaping destination.
func TestWriteSkillReplacesEscapingFileSymlink(t *testing.T) {
	target := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, "graphify"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	escaped := filepath.Join(outside, "escaped.md")
	if err := os.Symlink(escaped, filepath.Join(target, "graphify", "SKILL.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, err := writeSkill(target, "graphify", "safe"); err != nil {
		t.Fatalf("writeSkill: %v", err)
	}
	// Nothing written through the symlink to outside the target.
	if _, err := os.Stat(escaped); err == nil {
		t.Error("content was written outside the target via the file symlink")
	}
	// SKILL.md is now a regular file inside the target with the content.
	real := filepath.Join(target, "graphify", "SKILL.md")
	fi, err := os.Lstat(real)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Error("SKILL.md is still a symlink; want a regular file")
	}
	got, _ := os.ReadFile(real)
	if string(got) != "safe" {
		t.Errorf("content = %q, want %q", got, "safe")
	}
}

// A failed write must not truncate or destroy an already-installed skill: the
// atomic temp-then-rename keeps the existing SKILL.md intact.
func TestWriteSkillPreservesExistingOnFailure(t *testing.T) {
	target := t.TempDir()
	if _, err := writeSkill(target, "graphify", "original"); err != nil {
		t.Fatalf("first install: %v", err)
	}
	skillDir := filepath.Join(target, "graphify")
	// Make the skill dir read-only so creating the temp file fails.
	if err := os.Chmod(skillDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(skillDir, 0o755) })

	if _, err := writeSkill(target, "graphify", "replacement"); err == nil {
		t.Fatal("expected write failure into a read-only skill dir")
	}
	got, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read existing: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("existing skill corrupted on failed install: content = %q, want %q", got, "original")
	}
}
