package host

import "testing"

// TestNormalize verifies case, whitespace, and fully qualified host syntax are canonicalized.
func TestNormalize(t *testing.T) {
	tests := map[string]string{
		" EXAMPLE.COM. ":      "example.com",
		"*.APPS.EXAMPLE.COM.": "*.apps.example.com",
		"coder-rbac":          "coder-rbac",
		"":                    "",
	}
	for input, expected := range tests {
		if got := Normalize(input); got != expected {
			t.Errorf("Normalize(%q) = %q, want %q", input, got, expected)
		}
	}
}

// TestWithoutPort verifies hostnames are extracted without changing unbracketed hosts.
func TestWithoutPort(t *testing.T) {
	tests := map[string]string{
		"EXAMPLE.COM.:443": "example.com",
		"[::1]:8080":       "::1",
		"coder-rbac":       "coder-rbac",
		"example.com.":     "example.com",
	}
	for input, expected := range tests {
		if got := WithoutPort(input); got != expected {
			t.Errorf("WithoutPort(%q) = %q, want %q", input, got, expected)
		}
	}
}
