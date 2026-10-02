package webhooks

import (
	"context"
	"net"
	"net/url"
	"time"

	"siracrm/internal/domain"
)

// Resolver looks up webhook hostnames. Tests supply a fake; production uses the system resolver.
type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// Policy is applied when an endpoint is saved and again when a delivery connects.
type Policy struct {
	AllowLoopback bool
	Resolver      Resolver
}

func ValidateDestination(ctx context.Context, raw string, policy Policy) error {
	if err := domain.ValidateWebhookURL(raw, policy.AllowLoopback); err != nil {
		return err
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return domain.Invalid("Enter a valid webhook URL")
	}
	host := parsed.Hostname()
	if net.ParseIP(host) != nil {
		return nil
	}
	ips, err := policy.lookup(ctx, host)
	if err != nil || len(ips) == 0 {
		return domain.Invalid("Webhook URL could not be resolved")
	}
	for _, ip := range ips {
		if domain.BlockedIP(ip, policy.AllowLoopback) {
			return domain.Invalid("Webhook URLs must be public https addresses")
		}
	}
	return nil
}

// DialContext re-checks every resolved address and connects to that address, not the name.
func (policy Policy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := policy.lookup(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, domain.Invalid("Webhook URL could not be resolved")
	}
	for _, ip := range ips {
		if domain.BlockedIP(ip, policy.AllowLoopback) {
			return nil, domain.Invalid("Webhook URLs must be public https addresses")
		}
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

func (policy Policy) lookup(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	resolver := policy.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	return ips, nil
}
