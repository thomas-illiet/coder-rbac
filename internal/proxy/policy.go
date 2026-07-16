package proxy

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/thomas-illiet/coder-rbac/internal/coder"
)

// Decision describes whether and how the proxy must reject a request.
type Decision struct {
	Status  int
	Code    string
	Message string
	Detail  string
}

// Blocked reports whether the decision rejects the request.
func (d Decision) Blocked() bool { return d.Status != 0 }

// policyClient is the Coder API subset required to evaluate restricted operations.
type policyClient interface {
	CurrentUser(context.Context, *http.Request) (coder.User, error)
	Workspace(context.Context, *http.Request, uuid.UUID) (coder.Workspace, error)
	WorkspaceBuildParameters(context.Context, *http.Request, uuid.UUID) ([]coder.WorkspaceBuildParameter, error)
	TemplateVersionParameters(context.Context, *http.Request, uuid.UUID) ([]coder.TemplateVersionParameter, error)
	TemplateVersionPresets(context.Context, *http.Request, uuid.UUID) ([]coder.Preset, error)
}

// Policy evaluates restricted Coder operations for users outside the allowlist.
type Policy struct {
	client           policyClient
	allowedUsernames map[string]struct{}
	maxRequestBytes  int64
}

// NewPolicy creates a policy evaluator with allowed usernames and an inspection limit.
func NewPolicy(client policyClient, allowedUsernames map[string]struct{}, maxRequestBytes int64) *Policy {
	return &Policy{client: client, allowedUsernames: allowedUsernames, maxRequestBytes: maxRequestBytes}
}

// Evaluate returns the decision for an authenticated request and normalized path.
func (p *Policy) Evaluate(ctx context.Context, request *http.Request, normalizedPath string) Decision {
	route := classifyRoute(request.Method, normalizedPath)
	if route.kind == routeAllowed {
		return Decision{}
	}

	user, err := p.client.CurrentUser(ctx, request)
	if err != nil {
		var apiErr *coder.APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			return Decision{Status: apiErr.StatusCode, Code: "authentication_required", Message: "Coder authentication is required."}
		}
		return Decision{Status: http.StatusServiceUnavailable, Code: "identity_unavailable", Message: "Unable to verify the Coder identity.", Detail: err.Error()}
	}
	if _, ok := p.allowedUsernames[user.Username]; ok {
		return Decision{}
	}

	switch route.kind {
	case routeCreateWorkspace:
		return forbidden("workspace_creation_forbidden", "Workspace creation is managed by an external service.")
	case routeRenameWorkspace:
		return forbidden("workspace_rename_forbidden", "Workspace renaming is not allowed.")
	case routeWorkspaceSetting:
		return forbidden("workspace_settings_forbidden", "This workspace setting is managed externally.")
	case routeDormancy:
		return p.evaluateDormancy(request)
	case routeWorkspaceBuild:
		return p.evaluateBuild(ctx, request, route.workspaceID)
	default:
		return Decision{}
	}
}

// policyDependencyError creates a fail-closed decision for unavailable Coder policy data.
func policyDependencyError(name string, err error) Decision {
	return Decision{Status: http.StatusServiceUnavailable, Code: "policy_data_unavailable", Message: "Unable to validate " + name + ".", Detail: err.Error()}
}

// forbidden creates a consistent HTTP 403 policy decision.
func forbidden(code, message string) Decision {
	return Decision{Status: http.StatusForbidden, Code: code, Message: message}
}
