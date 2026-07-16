package proxy

import (
	"net/http"
	"testing"
)

// TestStaticPolicyRoutes verifies method and route restrictions that need no body inspection.
func TestStaticPolicyRoutes(t *testing.T) {
	fixture := newPolicyFixture(t, nil)
	tests := []struct {
		name    string
		method  string
		path    string
		blocked bool
	}{
		{name: "create", method: http.MethodPost, path: "/api/v2/users/me/workspaces", blocked: true},
		{name: "legacy organization creation explicitly allowed", method: http.MethodPost, path: "/api/v2/organizations/default/members/me/workspaces", blocked: false},
		{name: "rename", method: http.MethodPatch, path: "/api/v2/workspaces/" + testWorkspaceID.String(), blocked: true},
		{name: "autostart", method: http.MethodPut, path: "/api/v2/workspaces/" + testWorkspaceID.String() + "/autostart", blocked: true},
		{name: "autoupdates", method: http.MethodPut, path: "/api/v2/workspaces/" + testWorkspaceID.String() + "/autoupdates", blocked: true},
		{name: "extend", method: http.MethodPut, path: "/api/v2/workspaces/" + testWorkspaceID.String() + "/extend", blocked: true},
		{name: "ttl", method: http.MethodPut, path: "/api/v2/workspaces/" + testWorkspaceID.String() + "/ttl", blocked: true},
		{name: "state", method: http.MethodPut, path: "/api/v2/workspacebuilds/" + testBuildID.String() + "/state", blocked: true},
		{name: "favorite", method: http.MethodPut, path: "/api/v2/workspaces/" + testWorkspaceID.String() + "/favorite", blocked: false},
		{name: "acl", method: http.MethodPatch, path: "/api/v2/workspaces/" + testWorkspaceID.String() + "/acl", blocked: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := fixture.evaluate(test.method, test.path, `{}`)
			if decision.Blocked() != test.blocked {
				t.Fatalf("blocked = %v (%+v), want %v", decision.Blocked(), decision, test.blocked)
			}
		})
	}
	if fixture.seenToken != "secret-test-token" {
		t.Fatalf("identity token = %q, want forwarded token", fixture.seenToken)
	}
}
