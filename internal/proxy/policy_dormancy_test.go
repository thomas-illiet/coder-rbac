package proxy

import (
	"net/http"
	"testing"
)

// TestDormancyPolicy verifies waking is allowed while marking dormant is blocked.
func TestDormancyPolicy(t *testing.T) {
	fixture := newPolicyFixture(t, nil)
	path := "/api/v2/workspaces/" + testWorkspaceID.String() + "/dormant"
	if decision := fixture.evaluate(http.MethodPut, path, `{"dormant":true}`); decision.Code != "workspace_dormancy_forbidden" {
		t.Fatalf("true decision = %+v", decision)
	}
	if decision := fixture.evaluate(http.MethodPut, path, `{"dormant":false}`); decision.Blocked() {
		t.Fatalf("false decision = %+v", decision)
	}
}

// TestDormancyPolicyRejectsInvalidBodies verifies missing, malformed, and trailing data fail closed.
func TestDormancyPolicyRejectsInvalidBodies(t *testing.T) {
	path := "/api/v2/workspaces/" + testWorkspaceID.String() + "/dormant"
	for name, body := range map[string]string{
		"empty":         "",
		"malformed":     `{`,
		"missing field": `{}`,
		"null field":    `{"dormant":null}`,
		"trailing JSON": `{"dormant":false} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPolicyFixture(t, nil)
			decision := fixture.evaluate(http.MethodPut, path, body)
			if decision.Status != http.StatusBadRequest || decision.Code != "invalid_request" {
				t.Fatalf("decision = %+v", decision)
			}
		})
	}
}
