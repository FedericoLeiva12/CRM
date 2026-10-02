package config

import "testing"

func TestValidateOrigin(t *testing.T) {
	for _, origin := range []string{"https://crm.example.com", "http://localhost:3000", "http://127.0.0.1:3000"} {
		if err := ValidateOrigin(origin); err != nil {
			t.Errorf("Rejected valid origin %q: %v", origin, err)
		}
	}
	for _, origin := range []string{"", "http://crm.example.com", "http://localhost.attacker.test", "https://crm.example.com/", "https://crm.example.com/path", "https://user:password@crm.example.com", "https://crm.example.com?query=x"} {
		if ValidateOrigin(origin) == nil {
			t.Errorf("Accepted unsafe origin %q", origin)
		}
	}
}
