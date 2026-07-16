package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/thomas-illiet/coder-rbac/internal/coder"
)

var (
	testWorkspaceID    = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	testBuildID        = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	testCurrentVersion = uuid.MustParse("44444444-4444-4444-4444-444444444444")
	testTargetVersion  = uuid.MustParse("55555555-5555-5555-5555-555555555555")
	testPresetID       = uuid.MustParse("66666666-6666-6666-6666-666666666666")
)

const testUsername = "platform-admin"

// policyFixture provides deterministic Coder API data and a policy evaluator for tests.
type policyFixture struct {
	t                *testing.T
	server           *httptest.Server
	policy           *Policy
	workspace        coder.Workspace
	currentSchema    []coder.TemplateVersionParameter
	targetSchema     []coder.TemplateVersionParameter
	currentValues    []coder.WorkspaceBuildParameter
	presetValues     []coder.WorkspaceBuildParameter
	identityStatus   int
	identityUsername string
	dependencyStatus map[string]int
	seenToken        string
}

// newPolicyFixture creates a policy test server with deterministic Coder API responses.
func newPolicyFixture(t *testing.T, allowed map[string]struct{}) *policyFixture {
	t.Helper()
	fixture := &policyFixture{
		t:                t,
		workspace:        coder.Workspace{ID: testWorkspaceID, LatestBuild: coder.WorkspaceBuild{ID: testBuildID, TemplateVersionID: testCurrentVersion}},
		currentSchema:    []coder.TemplateVersionParameter{{Name: "cpu"}, {Name: "debug", Ephemeral: true}},
		targetSchema:     []coder.TemplateVersionParameter{{Name: "cpu"}, {Name: "debug", Ephemeral: true}},
		currentValues:    []coder.WorkspaceBuildParameter{{Name: "cpu", Value: "2"}, {Name: "debug", Value: "false"}},
		identityStatus:   http.StatusOK,
		identityUsername: testUsername,
		dependencyStatus: make(map[string]int),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	baseURL, _ := url.Parse(fixture.server.URL)
	fixture.policy = NewPolicy(coder.NewClient(baseURL, fixture.server.Client()), allowed, 1<<20)
	t.Cleanup(fixture.server.Close)
	return fixture
}

// handle serves the Coder API subset required by policy tests.
func (f *policyFixture) handle(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	if request.URL.Path == "/api/v2/users/me" {
		f.seenToken = request.Header.Get("Coder-Session-Token")
		if f.identityStatus != http.StatusOK {
			response.WriteHeader(f.identityStatus)
			return
		}
		writeTestJSON(f.t, response, coder.User{Username: f.identityUsername})
		return
	}
	if status := f.dependencyStatus[request.URL.Path]; status != 0 {
		response.WriteHeader(status)
		return
	}
	switch request.URL.Path {
	case "/api/v2/workspaces/" + testWorkspaceID.String():
		writeTestJSON(f.t, response, f.workspace)
	case "/api/v2/workspacebuilds/" + testBuildID.String() + "/parameters":
		writeTestJSON(f.t, response, f.currentValues)
	case "/api/v2/templateversions/" + testCurrentVersion.String() + "/rich-parameters":
		writeTestJSON(f.t, response, f.currentSchema)
	case "/api/v2/templateversions/" + testTargetVersion.String() + "/rich-parameters":
		writeTestJSON(f.t, response, f.targetSchema)
	case "/api/v2/templateversions/" + testTargetVersion.String() + "/presets",
		"/api/v2/templateversions/" + testCurrentVersion.String() + "/presets":
		writeTestJSON(f.t, response, []coder.Preset{{ID: testPresetID, Parameters: f.presetValues}})
	default:
		http.NotFound(response, request)
	}
}

// writeTestJSON serializes a fixture response and fails the test on encoding errors.
func writeTestJSON(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

// evaluate constructs a request and returns the fixture policy decision.
func (f *policyFixture) evaluate(method, path, body string) Decision {
	f.t.Helper()
	request := httptest.NewRequest(method, "http://coder.example.com"+path, strings.NewReader(body))
	request.Header.Set("Coder-Session-Token", "secret-test-token")
	normalized, err := NormalizePath(request)
	if err != nil {
		f.t.Fatal(err)
	}
	return f.policy.Evaluate(request.Context(), request, normalized)
}

// TestAllowedUsernameBypassesPolicy verifies a canonical username bypasses every restriction.
func TestAllowedUsernameBypassesPolicy(t *testing.T) {
	fixture := newPolicyFixture(t, map[string]struct{}{testUsername: {}})
	decision := fixture.evaluate(http.MethodPatch, "/api/v2/workspaces/"+testWorkspaceID.String(), `{}`)
	if decision.Blocked() {
		t.Fatalf("allowed user blocked: %+v", decision)
	}
}

// TestAllowedUsernameMatchingIsCaseSensitive verifies differently cased identities stay restricted.
func TestAllowedUsernameMatchingIsCaseSensitive(t *testing.T) {
	fixture := newPolicyFixture(t, map[string]struct{}{"Platform-Admin": {}})
	decision := fixture.evaluate(http.MethodPatch, "/api/v2/workspaces/"+testWorkspaceID.String(), `{}`)
	if decision.Status != http.StatusForbidden || decision.Code != "workspace_rename_forbidden" {
		t.Fatalf("decision = %+v", decision)
	}
}

// TestMissingUsernameIsFailClosed verifies an incomplete identity cannot receive an exemption.
func TestMissingUsernameIsFailClosed(t *testing.T) {
	fixture := newPolicyFixture(t, map[string]struct{}{testUsername: {}})
	fixture.identityUsername = ""
	decision := fixture.evaluate(http.MethodPatch, "/api/v2/workspaces/"+testWorkspaceID.String(), `{}`)
	if decision.Status != http.StatusServiceUnavailable || decision.Code != "identity_unavailable" {
		t.Fatalf("decision = %+v", decision)
	}
}

// TestIdentityFailureIsFailClosed verifies identity lookup failures reject protected requests.
func TestIdentityFailureIsFailClosed(t *testing.T) {
	fixture := newPolicyFixture(t, nil)
	fixture.identityStatus = http.StatusUnauthorized
	decision := fixture.evaluate(http.MethodPatch, "/api/v2/workspaces/"+testWorkspaceID.String(), `{}`)
	if decision.Status != http.StatusUnauthorized || decision.Code != "authentication_required" {
		t.Fatalf("decision = %+v", decision)
	}
}
