package eth

import "testing"

func TestDeriveProviderLabel(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expect   string
	}{
		{
			name:     "host only",
			endpoint: "https://mainnet.infura.io/v3/key",
			expect:   "mainnet.infura.io",
		},
		{
			name:     "with credentials",
			endpoint: "https://user:pass@alchemy.example.com/path",
			expect:   "alchemy.example.com",
		},
		{
			name:     "invalid url falls back",
			endpoint: "not a url",
			expect:   "not a url",
		},
		{
			name:     "empty endpoint",
			endpoint: "",
			expect:   "",
		},
		{
			name:     "scheme without host",
			endpoint: "localhost:8545",
			expect:   "localhost:8545",
		},
	}

	for _, tt := range tests {
		if got := deriveProviderLabel(tt.endpoint); got != tt.expect {
			t.Errorf("%s: expected %q, got %q", tt.name, tt.expect, got)
		}
	}
}
