package api_test

import (
	"encoding/json"
	"io"
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

// The update path wraps slug validation independently; a noncanonical slug on
// PATCH must also be 400, not 500.
func TestUpdateSkillInvalidSlug(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Post(srv.URL+"/skills", "application/json",
		strings.NewReader(`{"name":"Graphify","content":"body","slug":"graphify"}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID == "" {
		t.Fatalf("decode create: %v (%s)", err, body)
	}

	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/skills/"+created.ID,
		strings.NewReader(`{"slug":"Not Canonical!"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	up, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	up.Body.Close()
	if up.StatusCode != http.StatusBadRequest {
		t.Errorf("PATCH noncanonical slug = %d, want 400", up.StatusCode)
	}
}
