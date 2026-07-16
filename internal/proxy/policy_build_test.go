package proxy

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/thomas-illiet/coder-rbac/internal/coder"
)

// TestBuildPolicy verifies transitions, upgrades, presets, and immutable workspace parameters.
func TestBuildPolicy(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		configure    func(*policyFixture)
		blocked      bool
		expectedCode string
	}{
		{name: "standard start", body: `{"transition":"start"}`, blocked: false},
		{name: "standard stop", body: `{"transition":"stop"}`, blocked: false},
		{name: "delete", body: `{"transition":"delete"}`, blocked: true, expectedCode: "workspace_deletion_forbidden"},
		{name: "state override", body: `{"transition":"start","state":"c2VjcmV0"}`, blocked: true, expectedCode: "workspace_state_forbidden"},
		{name: "same persistent value", body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"2"}]}`, blocked: false},
		{name: "changed persistent value", body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"8"}]}`, blocked: true, expectedCode: "workspace_parameter_change_forbidden"},
		{name: "same ephemeral value", body: `{"transition":"start","rich_parameter_values":[{"name":"debug","value":"false"}]}`, blocked: false},
		{name: "changed ephemeral value", body: `{"transition":"start","rich_parameter_values":[{"name":"debug","value":"true"}]}`, blocked: true, expectedCode: "workspace_parameter_change_forbidden"},
		{name: "submitted parameter without current value fails closed", body: `{"transition":"start","rich_parameter_values":[{"name":"debug","value":"false"}]}`, configure: func(f *policyFixture) {
			f.currentValues = []coder.WorkspaceBuildParameter{{Name: "cpu", Value: "2"}}
		}, blocked: true, expectedCode: "policy_data_unavailable"},
		{name: "persistent parameter without current value fails closed", body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"2"}]}`, configure: func(f *policyFixture) {
			f.currentValues = []coder.WorkspaceBuildParameter{{Name: "debug", Value: "false"}}
		}, blocked: true, expectedCode: "policy_data_unavailable"},
		{name: "upgrade same schema", body: `{"transition":"start","template_version_id":"` + testTargetVersion.String() + `"}`, blocked: false},
		{name: "upgrade adds persistent", body: `{"transition":"start","template_version_id":"` + testTargetVersion.String() + `"}`, configure: func(f *policyFixture) {
			f.targetSchema = append(f.targetSchema, coder.TemplateVersionParameter{Name: "memory"})
		}, blocked: true, expectedCode: "persistent_parameter_schema_changed"},
		{name: "upgrade removes persistent", body: `{"transition":"start","template_version_id":"` + testTargetVersion.String() + `"}`, configure: func(f *policyFixture) {
			f.targetSchema = []coder.TemplateVersionParameter{{Name: "debug", Ephemeral: true}}
		}, blocked: true, expectedCode: "persistent_parameter_schema_changed"},
		{name: "upgrade introduces ephemeral parameter", body: `{"transition":"start","template_version_id":"` + testTargetVersion.String() + `","rich_parameter_values":[{"name":"trace","value":"true"}]}`, configure: func(f *policyFixture) {
			f.targetSchema = append(f.targetSchema, coder.TemplateVersionParameter{Name: "trace", Ephemeral: true})
		}, blocked: false},
		{name: "preset changes persistent", body: `{"transition":"start","template_version_id":"` + testTargetVersion.String() + `","template_version_preset_id":"` + testPresetID.String() + `"}`, configure: func(f *policyFixture) {
			f.presetValues = []coder.WorkspaceBuildParameter{{Name: "cpu", Value: "8"}}
		}, blocked: true, expectedCode: "workspace_parameter_change_forbidden"},
		{name: "preset keeps ephemeral value", body: `{"transition":"start","template_version_id":"` + testTargetVersion.String() + `","template_version_preset_id":"` + testPresetID.String() + `"}`, configure: func(f *policyFixture) {
			f.presetValues = []coder.WorkspaceBuildParameter{{Name: "debug", Value: "false"}}
		}, blocked: false},
		{name: "preset changes only ephemeral", body: `{"transition":"start","template_version_id":"` + testTargetVersion.String() + `","template_version_preset_id":"` + testPresetID.String() + `"}`, configure: func(f *policyFixture) {
			f.presetValues = []coder.WorkspaceBuildParameter{{Name: "debug", Value: "true"}}
		}, blocked: true, expectedCode: "workspace_parameter_change_forbidden"},
		{name: "duplicate schema parameter fails closed", body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"2"}]}`, configure: func(f *policyFixture) {
			f.currentSchema = append(f.currentSchema, coder.TemplateVersionParameter{Name: "cpu"})
		}, blocked: true, expectedCode: "policy_data_unavailable"},
		{name: "empty schema parameter fails closed", body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"2"}]}`, configure: func(f *policyFixture) {
			f.currentSchema = append(f.currentSchema, coder.TemplateVersionParameter{})
		}, blocked: true, expectedCode: "policy_data_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPolicyFixture(t, nil)
			if test.configure != nil {
				test.configure(fixture)
			}
			path := "/api/v2/workspaces/" + testWorkspaceID.String() + "/builds"
			decision := fixture.evaluate(http.MethodPost, path, test.body)
			if decision.Blocked() != test.blocked {
				t.Fatalf("blocked = %v (%+v), want %v", decision.Blocked(), decision, test.blocked)
			}
			if test.expectedCode != "" && decision.Code != test.expectedCode {
				t.Fatalf("code = %q, want %q", decision.Code, test.expectedCode)
			}
		})
	}
}

// TestBuildPolicyRejectsInvalidRequests verifies malformed payloads and invalid workspace data fail closed.
func TestBuildPolicyRejectsInvalidRequests(t *testing.T) {
	missingPresetID := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	tests := []struct {
		name         string
		workspaceID  string
		body         string
		configure    func(*policyFixture)
		expectedCode string
	}{
		{name: "empty body", workspaceID: testWorkspaceID.String(), body: "", expectedCode: "invalid_request"},
		{name: "malformed body", workspaceID: testWorkspaceID.String(), body: `{`, expectedCode: "invalid_request"},
		{name: "trailing JSON", workspaceID: testWorkspaceID.String(), body: `{"transition":"start"} {}`, expectedCode: "invalid_request"},
		{name: "missing transition", workspaceID: testWorkspaceID.String(), body: `{}`, expectedCode: "invalid_request"},
		{name: "unsupported transition", workspaceID: testWorkspaceID.String(), body: `{"transition":"restart"}`, expectedCode: "workspace_transition_forbidden"},
		{name: "invalid workspace ID", workspaceID: "not-a-uuid", body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"2"}]}`, expectedCode: "invalid_workspace"},
		{name: "missing build data", workspaceID: testWorkspaceID.String(), body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"2"}]}`, configure: func(f *policyFixture) {
			f.workspace.LatestBuild = coder.WorkspaceBuild{}
		}, expectedCode: "policy_data_unavailable"},
		{name: "missing preset", workspaceID: testWorkspaceID.String(), body: `{"transition":"start","template_version_preset_id":"` + missingPresetID.String() + `"}`, expectedCode: "invalid_template_preset"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPolicyFixture(t, nil)
			if test.configure != nil {
				test.configure(fixture)
			}
			decision := fixture.evaluate(http.MethodPost, "/api/v2/workspaces/"+test.workspaceID+"/builds", test.body)
			if !decision.Blocked() || decision.Code != test.expectedCode {
				t.Fatalf("decision = %+v, want code %q", decision, test.expectedCode)
			}
		})
	}
}

// TestBuildPolicyEnforcesRequestSizeLimit verifies oversized inspected bodies receive HTTP 413.
func TestBuildPolicyEnforcesRequestSizeLimit(t *testing.T) {
	fixture := newPolicyFixture(t, nil)
	body := `{"transition":"start"}`
	fixture.policy.maxRequestBytes = int64(len(body) - 1)
	decision := fixture.evaluate(http.MethodPost, "/api/v2/workspaces/"+testWorkspaceID.String()+"/builds", body)
	if decision.Status != http.StatusRequestEntityTooLarge || decision.Code != "request_too_large" {
		t.Fatalf("decision = %+v", decision)
	}
}

// TestBuildPolicyDependencyFailures verifies every required Coder lookup fails closed.
func TestBuildPolicyDependencyFailures(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "workspace", path: "/api/v2/workspaces/" + testWorkspaceID.String(), body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"2"}]}`},
		{name: "current schema", path: "/api/v2/templateversions/" + testCurrentVersion.String() + "/rich-parameters", body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"2"}]}`},
		{name: "target schema", path: "/api/v2/templateversions/" + testTargetVersion.String() + "/rich-parameters", body: `{"transition":"start","template_version_id":"` + testTargetVersion.String() + `"}`},
		{name: "current values", path: "/api/v2/workspacebuilds/" + testBuildID.String() + "/parameters", body: `{"transition":"start","rich_parameter_values":[{"name":"cpu","value":"2"}]}`},
		{name: "presets", path: "/api/v2/templateversions/" + testCurrentVersion.String() + "/presets", body: `{"transition":"start","template_version_preset_id":"` + testPresetID.String() + `"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPolicyFixture(t, nil)
			fixture.dependencyStatus[test.path] = http.StatusInternalServerError
			decision := fixture.evaluate(http.MethodPost, "/api/v2/workspaces/"+testWorkspaceID.String()+"/builds", test.body)
			if decision.Status != http.StatusServiceUnavailable || decision.Code != "policy_data_unavailable" {
				t.Fatalf("decision = %+v", decision)
			}
		})
	}
}
