package domain

import (
	"errors"
	"testing"
)

func TestProjectValidate(t *testing.T) {
	base := Project{Name: "n", Slug: "n", RepoPath: "/abs/repo"}
	if err := base.Validate(); err != nil {
		t.Errorf("valid project rejected: %v", err)
	}

	cases := map[string]Project{
		"blank name":    {Name: " ", Slug: "n", RepoPath: "/abs/repo"},
		"blank repo":    {Name: "n", Slug: "n", RepoPath: ""},
		"relative repo": {Name: "n", Slug: "n", RepoPath: "repo/x"},
		"missing slug":  {Name: "n", Slug: "", RepoPath: "/abs/repo"},
	}
	for name, p := range cases {
		if err := p.Validate(); !errors.Is(err, ErrInvalidProject) {
			t.Errorf("%s: err = %v, want ErrInvalidProject", name, err)
		}
	}
}

func TestValidateSlug(t *testing.T) {
	if err := ValidateSlug("good-slug"); err != nil {
		t.Errorf("canonical slug rejected: %v", err)
	}
	for _, bad := range []string{"", "   ", "Not Canonical!", "Has Space"} {
		if err := ValidateSlug(bad); !errors.Is(err, ErrInvalidSlug) {
			t.Errorf("ValidateSlug(%q) = %v, want ErrInvalidSlug", bad, err)
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World":  "hello-world",
		"  Foo_Bar!! ": "foo-bar",
		"already-slug": "already-slug",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEncodeCWD(t *testing.T) {
	if got := EncodeCWD("/Users/brent/Projects/x"); got != "-Users-brent-Projects-x" {
		t.Errorf("EncodeCWD = %q", got)
	}
}
