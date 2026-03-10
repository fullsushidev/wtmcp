package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// safeDialer wraps net.Dialer to reject connections to private,
// loopback, link-local, and unspecified IP addresses at connection
// time. This prevents SSRF via DNS rebinding — the IP validation
// happens on the same DNS resolution used for the actual connection,
// eliminating the TOCTOU window of a separate pre-check.
type safeDialer struct {
	dialer       net.Dialer
	allowPrivate bool // for testing only
}

// DialContext resolves the host, validates all IPs against the
// private/loopback denylist, then connects to the first valid IP.
func (d *safeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}

	if !d.allowPrivate {
		for _, ipStr := range ips {
			if err := checkIP(ipStr); err != nil {
				return nil, fmt.Errorf("SSRF blocked: host %s: %w", host, err)
			}
		}
	}

	// Connect using the validated IPs directly
	var lastErr error
	for _, ipStr := range ips {
		conn, err := d.dialer.DialContext(ctx, network, net.JoinHostPort(ipStr, port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("connect to %s: %w", addr, lastErr)
}

// checkIP rejects private, loopback, link-local, and unspecified
// addresses, including IPv6-mapped IPv4 representations.
func checkIP(ipStr string) error {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("unparseable IP address %q", ipStr)
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("resolves to private/loopback address %s", ipStr)
	}

	// IPv6-mapped IPv4 (::ffff:x.x.x.x): Go's IsPrivate only checks
	// IPv4 ranges against 4-byte IPs. A 16-byte representation like
	// ::ffff:10.0.0.1 would slip through the check above.
	if ipv4 := ip.To4(); ipv4 != nil && len(ip) == net.IPv6len {
		if ipv4.IsLoopback() || ipv4.IsPrivate() || ipv4.IsLinkLocalUnicast() || ipv4.IsUnspecified() {
			return fmt.Errorf("resolves to private/loopback address %s", ipStr)
		}
	}

	return nil
}

// safeTransport returns an http.Transport that uses a safeDialer
// to reject connections to private/loopback IPs.
func safeTransport(allowPrivate bool) *http.Transport {
	return &http.Transport{
		DialContext:           (&safeDialer{dialer: net.Dialer{Timeout: 30 * time.Second}, allowPrivate: allowPrivate}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
