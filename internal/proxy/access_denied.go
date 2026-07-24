package proxy

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/http"
	"strings"
)

var (
	//go:embed access_denied.html
	accessDeniedTemplateSource string

	//go:embed access_denied.css
	accessDeniedStyle string

	accessDeniedTemplate = template.Must(template.New("access-denied").Parse(accessDeniedTemplateSource))
)

// acceptsHTML reports whether a browser-style request explicitly accepts HTML.
func acceptsHTML(request *http.Request) bool {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}
	for _, header := range request.Header.Values("Accept") {
		for mediaRange := range strings.SplitSeq(header, ",") {
			mediaType := strings.TrimSpace(strings.SplitN(mediaRange, ";", 2)[0])
			if strings.EqualFold(mediaType, "text/html") || strings.EqualFold(mediaType, "application/xhtml+xml") {
				return true
			}
		}
	}
	return false
}

// writeAccessDenied renders the themed browser response without external assets.
func writeAccessDenied(response http.ResponseWriter, request *http.Request, loginURL string) error {
	var body bytes.Buffer
	err := accessDeniedTemplate.Execute(&body, struct {
		Host     string
		LoginURL string
		RetryURL string
		Style    template.CSS
	}{
		Host:     request.Host,
		LoginURL: loginURL,
		RetryURL: request.URL.RequestURI(),
		Style:    template.CSS(accessDeniedStyle),
	})
	if err != nil {
		return err
	}

	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(http.StatusForbidden)
	if request.Method != http.MethodHead {
		_, err = response.Write(body.Bytes())
	}
	return err
}
