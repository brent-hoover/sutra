package main_test

// Generated from docs/arch-rules.yaml — regenerate, don't hand-edit.

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

const modulePrefix = "github.com/brent-hoover/sutra/internal/"

var mayImport = map[string][]string{
	"domain":  {},
	"config":  {},
	"store":   {"domain", "config"},
	"service": {"domain", "store", "config"},
	"api":     {"domain", "service", "config"},
	"client":  {"domain", "config"},
	"tui":     {"domain", "client"},
	"cli":     {"domain", "client", "api", "config"},
}

func TestArchitecture(t *testing.T) {
	out, err := exec.Command("go", "list", "-json", "./internal/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	seen := map[string]bool{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var pkg struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&pkg); err != nil {
			break
		}
		mod := strings.SplitN(strings.TrimPrefix(pkg.ImportPath, modulePrefix), "/", 2)[0]
		seen[mod] = true
		allowedList, declared := mayImport[mod]
		if !declared {
			t.Errorf("undeclared module %q (%s) — not in arch-rules.yaml", mod, pkg.ImportPath)
			continue
		}
		allowed := map[string]bool{}
		for _, m := range allowedList {
			allowed[m] = true
		}
		for _, imp := range pkg.Imports {
			if !strings.HasPrefix(imp, modulePrefix) {
				continue
			}
			target := strings.SplitN(strings.TrimPrefix(imp, modulePrefix), "/", 2)[0]
			if target != mod && !allowed[target] {
				t.Errorf("%s imports %s — not in may_import[%q]", pkg.ImportPath, imp, mod)
			}
		}
	}
	for mod := range mayImport {
		if !seen[mod] {
			t.Errorf("declared module %q has no package under internal/", mod)
		}
	}
}
