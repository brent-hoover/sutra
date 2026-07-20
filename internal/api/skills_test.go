package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// A skill with a non-canonical slug must be a 400 (ErrInvalidSkill), not a 500 —
// the slug validator's error is wrapped as ErrInvalidSkill for correct mapping.
func TestCreateSkillInvalidSlug(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid", `{"name":"Graphify","content":"body","slug":"graphify"}`, http.StatusCreated},
		{"noncanonical slug", `{"name":"Graphify","content":"body","slug":"Not Canonical!"}`, http.StatusBadRequest},
		{"missing content", `{"name":"Graphify"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/skills", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
