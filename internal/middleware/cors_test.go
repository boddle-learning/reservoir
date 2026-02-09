package middleware

import (
	"testing"
)

func TestParseAllowedOrigins(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  []string{"*"},
		},
		{
			name:  "wildcard",
			input: "*",
			want:  []string{"*"},
		},
		{
			name:  "single origin",
			input: "http://localhost:3000",
			want:  []string{"http://localhost:3000"},
		},
		{
			name:  "multiple origins",
			input: "http://localhost:3000,http://localhost:4000",
			want:  []string{"http://localhost:3000", "http://localhost:4000"},
		},
		{
			name:  "multiple origins with spaces",
			input: "http://localhost:3000 , http://localhost:4000 , http://localhost:5000",
			want:  []string{"http://localhost:3000", "http://localhost:4000", "http://localhost:5000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAllowedOrigins(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("ParseAllowedOrigins() length = %d, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseAllowedOrigins()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
