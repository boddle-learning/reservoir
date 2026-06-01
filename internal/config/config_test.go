package config

import "testing"

func TestTrustedProxyList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty means trust none", "", nil},
		{"whitespace only", "   ", nil},
		{"single cidr", "10.0.0.0/8", []string{"10.0.0.0/8"}},
		{"multiple with spaces and blanks", " 10.0.0.0/8 , , 172.16.0.0/12 ,", []string{"10.0.0.0/8", "172.16.0.0/12"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{TrustedProxies: tt.in}
			got := c.TrustedProxyList()
			if len(got) != len(tt.want) {
				t.Fatalf("TrustedProxyList() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("TrustedProxyList()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
