package middleware

import (
	"strings"
	"testing"
)

func TestRedactQuery(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantContain []string // substrings that must be present
		wantAbsent  []string // substrings that must NOT be present
	}{
		{
			name:        "magic-link token is redacted",
			raw:         "token=super-secret-value",
			wantContain: []string{"token=REDACTED"},
			wantAbsent:  []string{"super-secret-value"},
		},
		{
			name:        "oauth code is redacted",
			raw:         "code=auth-code-123&state=xyz",
			wantContain: []string{"code=REDACTED", "state=xyz"},
			wantAbsent:  []string{"auth-code-123"},
		},
		{
			name:        "non-sensitive params are preserved verbatim",
			raw:         "redirect_url=/dashboard&foo=bar",
			wantContain: []string{"redirect_url=/dashboard&foo=bar"},
		},
		{
			name:        "sensitive mixed with benign",
			raw:         "token=abc&redirect_url=/x",
			wantContain: []string{"token=REDACTED", "redirect_url=%2Fx"},
			wantAbsent:  []string{"token=abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactQuery(tt.raw)
			for _, c := range tt.wantContain {
				if !strings.Contains(got, c) {
					t.Errorf("redactQuery(%q) = %q, want to contain %q", tt.raw, got, c)
				}
			}
			for _, a := range tt.wantAbsent {
				if strings.Contains(got, a) {
					t.Errorf("redactQuery(%q) = %q, must not contain %q", tt.raw, got, a)
				}
			}
		})
	}
}

func TestRedactQuery_Empty(t *testing.T) {
	if got := redactQuery(""); got != "" {
		t.Errorf("redactQuery(\"\") = %q, want \"\"", got)
	}
}

func TestRedactQuery_Unparseable(t *testing.T) {
	// A stray %ZZ is an invalid percent-encoding; rather than risk logging an
	// unparsed secret, redactQuery returns a fixed placeholder.
	got := redactQuery("token=%ZZ")
	if strings.Contains(got, "%ZZ") || got == "token=%ZZ" {
		t.Errorf("redactQuery returned the raw unparseable query: %q", got)
	}
}
