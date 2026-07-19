package proxy

import (
	"net/http"
	"strings"
)

// routeKind identifies the policy rule associated with a Coder API route.
type routeKind uint8

const (
	routeAllowed routeKind = iota
	routeCreateWorkspace
	routeRenameWorkspace
	routeWorkspaceSetting
	routeDormancy
	routeWorkspaceBuild
	routeUserPassword
	routeUserProfile
)

// routeMatch contains the matched rule and any workspace identifier captured from the path.
type routeMatch struct {
	kind        routeKind
	workspaceID string
}

// classifyRoute maps a method and normalized path to the policy rule that protects it.
func classifyRoute(method, normalizedPath string) routeMatch {
	segments := strings.Split(strings.Trim(normalizedPath, "/"), "/")
	if len(segments) < 2 || segments[0] != "api" || segments[1] != "v2" {
		return routeMatch{}
	}
	if method == http.MethodPost && len(segments) == 5 && segments[2] == "users" && segments[4] == "workspaces" {
		return routeMatch{kind: routeCreateWorkspace}
	}
	if method == http.MethodPut && len(segments) == 5 && segments[2] == "users" {
		switch segments[4] {
		case "password":
			return routeMatch{kind: routeUserPassword}
		case "profile":
			return routeMatch{kind: routeUserProfile}
		}
	}
	if len(segments) == 4 && segments[2] == "workspaces" && method == http.MethodPatch {
		return routeMatch{kind: routeRenameWorkspace}
	}
	if len(segments) == 5 && segments[2] == "workspaces" && method == http.MethodPut {
		switch segments[4] {
		case "autostart", "autoupdates", "extend", "ttl":
			return routeMatch{kind: routeWorkspaceSetting}
		case "dormant":
			return routeMatch{kind: routeDormancy}
		}
	}
	if len(segments) == 5 && segments[2] == "workspacebuilds" && segments[4] == "state" && method == http.MethodPut {
		return routeMatch{kind: routeWorkspaceSetting}
	}
	if len(segments) == 5 && segments[2] == "workspaces" && segments[4] == "builds" && method == http.MethodPost {
		return routeMatch{kind: routeWorkspaceBuild, workspaceID: segments[3]}
	}
	return routeMatch{}
}
