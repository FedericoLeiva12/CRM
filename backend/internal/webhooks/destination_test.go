package webhooks

import (
	"context"
	"net"
	"testing"
)

func TestDestinationDNS(t *testing.T) {
	resolver := stubResolver{ips: map[string][]net.IP{
		"public.test": {net.ParseIP("1.2.3.4")},
		"rebind.test": {net.ParseIP("1.2.3.4"), net.ParseIP("10.0.0.8")},
		"loop.test":   {net.ParseIP("127.0.0.1")},
		"local.test":  {net.ParseIP("169.254.169.254")},
	}}
	ctx := context.Background()
	policy := Policy{Resolver: resolver}
	if err := ValidateDestination(ctx, "https://public.test/hook", policy); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"https://rebind.test/hook", "https://loop.test/hook", "https://local.test/hook"} {
		if err := ValidateDestination(ctx, host, policy); err == nil {
			t.Fatalf("allowed %s", host)
		}
	}
	if _, err := policy.DialContext(ctx, "tcp", "rebind.test:443"); err == nil {
		t.Fatal("dial accepted a name that resolves to a private address")
	}
	if err := ValidateDestination(ctx, "https://loop.test/hook", Policy{AllowLoopback: true, Resolver: resolver}); err != nil {
		t.Fatal(err)
	}
}

type stubResolver struct {
	ips map[string][]net.IP
}

func (resolver stubResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	found := resolver.ips[host]
	addrs := make([]net.IPAddr, 0, len(found))
	for _, ip := range found {
		addrs = append(addrs, net.IPAddr{IP: ip})
	}
	return addrs, nil
}
