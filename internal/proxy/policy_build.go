package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/thomas-illiet/coder-rbac/internal/coder"
)

// buildRequest contains the workspace-build fields inspected by the policy.
type buildRequest struct {
	TemplateVersionID       uuid.UUID                       `json:"template_version_id"`
	TemplateVersionPresetID uuid.UUID                       `json:"template_version_preset_id"`
	Transition              string                          `json:"transition"`
	RichParameterValues     []coder.WorkspaceBuildParameter `json:"rich_parameter_values"`
	State                   json.RawMessage                 `json:"state"`
}

// buildPolicyData contains the current and target parameter data used for comparison.
type buildPolicyData struct {
	currentVersionID uuid.UUID
	targetVersionID  uuid.UUID
	currentKinds     map[string]bool
	targetKinds      map[string]bool
	currentValues    map[string]string
}

// evaluateBuild orchestrates payload validation, policy-data loading, preset resolution, and value comparison.
func (p *Policy) evaluateBuild(ctx context.Context, request *http.Request, rawWorkspaceID string) Decision {
	payload, decision := p.decodeBuildRequest(request)
	if decision.Blocked() {
		return decision
	}
	if decision := validateBuildRequest(payload); decision.Blocked() {
		return decision
	}
	if !requiresParameterValidation(payload) {
		return Decision{}
	}

	workspaceID, err := uuid.Parse(rawWorkspaceID)
	if err != nil {
		return Decision{Status: http.StatusBadRequest, Code: "invalid_workspace", Message: "The workspace identifier must be a UUID."}
	}
	data, decision := p.loadBuildPolicyData(ctx, request, workspaceID, payload.TemplateVersionID)
	if decision.Blocked() {
		return decision
	}
	submitted, decision := p.resolveSubmittedParameters(ctx, request, data.targetVersionID, payload)
	if decision.Blocked() {
		return decision
	}
	return validateSubmittedParameters(submitted, data)
}

// decodeBuildRequest reads and validates the JSON shape of a workspace-build request.
func (p *Policy) decodeBuildRequest(request *http.Request) (buildRequest, Decision) {
	body, decision := p.readBody(request)
	if decision.Blocked() {
		return buildRequest{}, decision
	}
	var payload buildRequest
	if err := decodeStrictJSON(body, &payload); err != nil {
		return buildRequest{}, Decision{Status: http.StatusBadRequest, Code: "invalid_request", Message: "Invalid workspace build request.", Detail: err.Error()}
	}
	return payload, Decision{}
}

// validateBuildRequest enforces transition and provisioner-state restrictions.
func validateBuildRequest(payload buildRequest) Decision {
	switch payload.Transition {
	case "delete":
		return forbidden("workspace_deletion_forbidden", "Workspace deletion is managed by an external service.")
	case "start", "stop":
	case "":
		return Decision{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The transition field is required."}
	default:
		return forbidden("workspace_transition_forbidden", "This workspace transition is not allowed by the pinned policy.")
	}
	if rawJSONHasValue(payload.State) {
		return forbidden("workspace_state_forbidden", "Provisioner state overrides are not allowed.")
	}
	return Decision{}
}

// requiresParameterValidation reports whether the request can change template-derived values.
func requiresParameterValidation(payload buildRequest) bool {
	return payload.TemplateVersionID != uuid.Nil || payload.TemplateVersionPresetID != uuid.Nil || len(payload.RichParameterValues) > 0
}

// loadBuildPolicyData retrieves and validates current and target parameter policy data.
func (p *Policy) loadBuildPolicyData(ctx context.Context, request *http.Request, workspaceID, requestedVersionID uuid.UUID) (buildPolicyData, Decision) {
	workspace, err := p.client.Workspace(ctx, request, workspaceID)
	if err != nil {
		return buildPolicyData{}, policyDependencyError("workspace", err)
	}
	if workspace.LatestBuild.ID == uuid.Nil || workspace.LatestBuild.TemplateVersionID == uuid.Nil {
		return buildPolicyData{}, Decision{Status: http.StatusServiceUnavailable, Code: "policy_data_unavailable", Message: "The workspace has no build data for parameter validation."}
	}

	targetVersionID := requestedVersionID
	if targetVersionID == uuid.Nil {
		targetVersionID = workspace.LatestBuild.TemplateVersionID
	}
	currentSchema, err := p.client.TemplateVersionParameters(ctx, request, workspace.LatestBuild.TemplateVersionID)
	if err != nil {
		return buildPolicyData{}, policyDependencyError("current template parameters", err)
	}
	targetSchema := currentSchema
	if targetVersionID != workspace.LatestBuild.TemplateVersionID {
		targetSchema, err = p.client.TemplateVersionParameters(ctx, request, targetVersionID)
		if err != nil {
			return buildPolicyData{}, policyDependencyError("target template parameters", err)
		}
	}
	currentKinds, currentPersistent, err := parameterKinds(currentSchema)
	if err != nil {
		return buildPolicyData{}, policyDependencyError("current template parameters", err)
	}
	targetKinds, targetPersistent, err := parameterKinds(targetSchema)
	if err != nil {
		return buildPolicyData{}, policyDependencyError("target template parameters", err)
	}
	if !sameNames(currentPersistent, targetPersistent) {
		return buildPolicyData{}, forbidden("persistent_parameter_schema_changed", "The target template version changes the persistent workspace parameter set.")
	}

	currentValuesList, err := p.client.WorkspaceBuildParameters(ctx, request, workspace.LatestBuild.ID)
	if err != nil {
		return buildPolicyData{}, policyDependencyError("current workspace parameters", err)
	}
	currentValues := make(map[string]string, len(currentValuesList))
	for _, parameter := range currentValuesList {
		currentValues[parameter.Name] = parameter.Value
	}
	for name := range currentPersistent {
		if _, ok := currentValues[name]; !ok {
			return buildPolicyData{}, Decision{Status: http.StatusServiceUnavailable, Code: "policy_data_unavailable", Message: "A persistent parameter has no current value.", Detail: "missing value for " + name}
		}
	}
	return buildPolicyData{
		currentVersionID: workspace.LatestBuild.TemplateVersionID,
		targetVersionID:  targetVersionID,
		currentKinds:     currentKinds,
		targetKinds:      targetKinds,
		currentValues:    currentValues,
	}, Decision{}
}

// resolveSubmittedParameters combines a selected preset with explicit parameter values.
func (p *Policy) resolveSubmittedParameters(ctx context.Context, request *http.Request, targetVersionID uuid.UUID, payload buildRequest) ([]coder.WorkspaceBuildParameter, Decision) {
	var submitted []coder.WorkspaceBuildParameter
	if payload.TemplateVersionPresetID != uuid.Nil {
		presets, err := p.client.TemplateVersionPresets(ctx, request, targetVersionID)
		if err != nil {
			return nil, policyDependencyError("template presets", err)
		}
		for _, preset := range presets {
			if preset.ID == payload.TemplateVersionPresetID {
				submitted = append(submitted, preset.Parameters...)
				submitted = append(submitted, payload.RichParameterValues...)
				return submitted, Decision{}
			}
		}
		return nil, Decision{Status: http.StatusBadRequest, Code: "invalid_template_preset", Message: "The requested template preset does not exist."}
	}
	return append(submitted, payload.RichParameterValues...), Decision{}
}

// validateSubmittedParameters ensures every submitted value is known and unchanged.
func validateSubmittedParameters(submitted []coder.WorkspaceBuildParameter, data buildPolicyData) Decision {
	for _, parameter := range submitted {
		ephemeral, known := data.targetKinds[parameter.Name]
		if !known {
			return forbidden("workspace_parameter_change_forbidden", "Unknown workspace parameters are not accepted by the policy.")
		}
		currentValue, ok := data.currentValues[parameter.Name]
		if !ok {
			_, existedInCurrentSchema := data.currentKinds[parameter.Name]
			if data.targetVersionID != data.currentVersionID && ephemeral && !existedInCurrentSchema {
				continue
			}
			return Decision{Status: http.StatusServiceUnavailable, Code: "policy_data_unavailable", Message: "A submitted workspace parameter has no current value.", Detail: "missing value for " + parameter.Name}
		}
		if parameter.Value != currentValue {
			return forbidden("workspace_parameter_change_forbidden", "Workspace parameter values cannot be changed.")
		}
	}
	return Decision{}
}

// parameterKinds validates a schema and indexes parameter ephemerality and persistent names.
func parameterKinds(schema []coder.TemplateVersionParameter) (map[string]bool, map[string]struct{}, error) {
	kinds := make(map[string]bool, len(schema))
	persistent := make(map[string]struct{})
	for _, parameter := range schema {
		if parameter.Name == "" {
			return nil, nil, fmt.Errorf("template parameter has an empty name")
		}
		if _, exists := kinds[parameter.Name]; exists {
			return nil, nil, fmt.Errorf("duplicate template parameter %q", parameter.Name)
		}
		kinds[parameter.Name] = parameter.Ephemeral
		if !parameter.Ephemeral {
			persistent[parameter.Name] = struct{}{}
		}
	}
	return kinds, persistent, nil
}

// sameNames reports whether two string sets contain exactly the same names.
func sameNames(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for name := range left {
		if _, ok := right[name]; !ok {
			return false
		}
	}
	return true
}
