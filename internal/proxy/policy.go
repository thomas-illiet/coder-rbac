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

// UserAccess contains the verified user attributes needed by workspace policy checks.
type UserAccess struct {
	authenticated        bool
	admin                bool
	authenticationStatus int
}

// Policy evaluates Coder access and restricted workspace operations.
type Policy struct {
	client           policyClient
	adminUsernames   map[string]struct{}
	allowedUsernames map[string]struct{}
	maxRequestBytes  int64
}

// NewPolicy creates a policy evaluator with administrator and access lists.
func NewPolicy(client policyClient, adminUsernames, allowedUsernames map[string]struct{}, maxRequestBytes int64) *Policy {
	return &Policy{client: client, adminUsernames: adminUsernames, allowedUsernames: allowedUsernames, maxRequestBytes: maxRequestBytes}
}

// Authorize verifies that an authenticated Coder user may access the application.
func (p *Policy) Authorize(ctx context.Context, request *http.Request) (UserAccess, Decision) {
	user, err := p.client.CurrentUser(ctx, request)
	if err != nil {
		var apiErr *coder.APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			return UserAccess{authenticationStatus: apiErr.StatusCode}, Decision{}
		}
		return UserAccess{}, Decision{Status: http.StatusServiceUnavailable, Code: "identity_unavailable", Message: "Unable to verify the Coder identity.", Detail: err.Error()}
	}
	if _, ok := p.adminUsernames[user.Username]; ok {
		return UserAccess{authenticated: true, admin: true}, Decision{}
	}
	if _, ok := p.allowedUsernames[user.Username]; ok {
		return UserAccess{authenticated: true}, Decision{}
	}
	return UserAccess{}, forbidden("user_access_forbidden", "This user is not allowed to access Coder.")
}

// Evaluate returns the workspace-policy decision for an admitted request.
func (p *Policy) Evaluate(ctx context.Context, request *http.Request, normalizedPath string, access UserAccess) Decision {
	route := classifyRoute(request.Method, normalizedPath)
	if route.kind == routeAllowed || access.admin {
		return Decision{}
	}
	if !access.authenticated {
		status := access.authenticationStatus
		if status == 0 {
			status = http.StatusUnauthorized
		}
		return Decision{Status: status, Code: "authentication_required", Message: "Coder authentication is required."}
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
	case routeUserPassword:
		return forbidden("user_password_update_forbidden", "Changing user passwords is not allowed.")
	case routeUserProfile:
		return forbidden("user_profile_update_forbidden", "Changing user profiles is not allowed.")
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
