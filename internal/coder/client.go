// Package coder provides the minimal authenticated Coder API client required by policy checks.
package coder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

const maxAPIResponseBytes = 4 << 20

// APIError describes a non-successful HTTP response returned by Coder.
type APIError struct {
	StatusCode int
	Operation  string
}

// Error formats the failed Coder API operation and its HTTP status code.
func (e *APIError) Error() string {
	return fmt.Sprintf("coder API %s returned HTTP %d", e.Operation, e.StatusCode)
}

// Client reads the Coder API data required to evaluate proxy policies.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

// NewClient creates a Coder API client that uses baseURL and httpClient.
func NewClient(baseURL *url.URL, httpClient *http.Client) *Client {
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// User contains the canonical Coder username needed for allowlist evaluation.
type User struct {
	Username string `json:"username"`
}

// Workspace contains the latest build metadata used for parameter validation.
type Workspace struct {
	ID          uuid.UUID      `json:"id"`
	LatestBuild WorkspaceBuild `json:"latest_build"`
}

// WorkspaceBuild identifies a build and the template version it uses.
type WorkspaceBuild struct {
	ID                uuid.UUID `json:"id"`
	TemplateVersionID uuid.UUID `json:"template_version_id"`
}

// WorkspaceBuildParameter is a named parameter value stored on a workspace build.
type WorkspaceBuildParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// TemplateVersionParameter describes whether a template parameter is ephemeral.
type TemplateVersionParameter struct {
	Name      string `json:"name"`
	Ephemeral bool   `json:"ephemeral"`
}

// Preset contains a template-version preset and the values it submits.
type Preset struct {
	ID         uuid.UUID                 `json:"id"`
	Parameters []WorkspaceBuildParameter `json:"parameters"`
}

// BuildInfo contains the running Coder server version.
type BuildInfo struct {
	Version string `json:"version"`
}

// CurrentUser returns the Coder user authenticated by the source request.
func (c *Client) CurrentUser(ctx context.Context, source *http.Request) (User, error) {
	var user User
	if err := c.getJSON(ctx, source, "/api/v2/users/me", &user); err != nil {
		return User{}, err
	}
	if user.Username == "" || user.Username != strings.TrimSpace(user.Username) {
		return User{}, fmt.Errorf("coder API /api/v2/users/me returned an invalid username")
	}
	return user, nil
}

// Workspace returns policy-relevant metadata for the workspace identified by id.
func (c *Client) Workspace(ctx context.Context, source *http.Request, id uuid.UUID) (Workspace, error) {
	var workspace Workspace
	err := c.getJSON(ctx, source, "/api/v2/workspaces/"+id.String(), &workspace)
	return workspace, err
}

// WorkspaceBuildParameters returns the parameter values recorded on a workspace build.
func (c *Client) WorkspaceBuildParameters(ctx context.Context, source *http.Request, buildID uuid.UUID) ([]WorkspaceBuildParameter, error) {
	var parameters []WorkspaceBuildParameter
	err := c.getJSON(ctx, source, "/api/v2/workspacebuilds/"+buildID.String()+"/parameters", &parameters)
	return parameters, err
}

// TemplateVersionParameters returns the rich-parameter schema for a template version.
func (c *Client) TemplateVersionParameters(ctx context.Context, source *http.Request, versionID uuid.UUID) ([]TemplateVersionParameter, error) {
	var parameters []TemplateVersionParameter
	err := c.getJSON(ctx, source, "/api/v2/templateversions/"+versionID.String()+"/rich-parameters", &parameters)
	return parameters, err
}

// TemplateVersionPresets returns the presets defined by a template version.
func (c *Client) TemplateVersionPresets(ctx context.Context, source *http.Request, versionID uuid.UUID) ([]Preset, error) {
	var presets []Preset
	err := c.getJSON(ctx, source, "/api/v2/templateversions/"+versionID.String()+"/presets", &presets)
	return presets, err
}

// BuildInfo returns version information for the upstream Coder server.
func (c *Client) BuildInfo(ctx context.Context) (BuildInfo, error) {
	var info BuildInfo
	err := c.getJSON(ctx, nil, "/api/v2/buildinfo", &info)
	return info, err
}

// getJSON performs an authenticated, size-limited JSON request against Coder.
func (c *Client) getJSON(ctx context.Context, source *http.Request, apiPath string, target any) error {
	destination := *c.baseURL
	destination.Path = strings.TrimSuffix(destination.Path, "/") + apiPath
	destination.RawPath = ""
	destination.RawQuery = ""

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, destination.String(), nil)
	if err != nil {
		return fmt.Errorf("create Coder API request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if source != nil {
		copyAuthenticationHeaders(request.Header, source.Header)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call Coder API %s: %w", apiPath, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return &APIError{StatusCode: response.StatusCode, Operation: apiPath}
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		return fmt.Errorf("coder API %s returned non-JSON content", apiPath)
	}

	payload, err := io.ReadAll(io.LimitReader(response.Body, maxAPIResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read Coder API %s: %w", apiPath, err)
	}
	if len(payload) > maxAPIResponseBytes {
		return fmt.Errorf("coder API %s response exceeds %d bytes", apiPath, maxAPIResponseBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode Coder API %s: %w", apiPath, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode Coder API %s: additional JSON value", apiPath)
		}
		return fmt.Errorf("decode Coder API %s trailing data: %w", apiPath, err)
	}
	return nil
}

// copyAuthenticationHeaders forwards only headers that can authenticate a Coder request.
func copyAuthenticationHeaders(destination, source http.Header) {
	for _, name := range []string{"Authorization", "Coder-Session-Token", "Cookie"} {
		for _, value := range source.Values(name) {
			destination.Add(name, value)
		}
	}
}
