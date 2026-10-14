// Package connector makes outbound HTTP requests on behalf of a workspace:
// executing approved calls against connections whose credentials Veto holds,
// and delivering webhooks. All of it goes through a transport that refuses
// private, loopback and link-local destinations unless the operator allows
// them, so a misconfigured URL cannot be used to reach internal services.
package connector

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a request resolves to a forbidden IP.
var ErrBlockedAddress = errors.New("connector: destination address is not allowed")

var blockedNets = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
		"172.16.0.0/12", "192.0.0.0/24", "192.168.0.0/16", "198.18.0.0/15", "224.0.0.0/4", "240.0.0.0/4",
		"::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
	}
	out := make([]*net.IPNet, len(cidrs))
	for i, c := range cidrs {
		_, n, _ := net.ParseCIDR(c)
		out[i] = n
	}
	return out
}()

// Blocked reports whether an IP is in a range that public requests must not
// reach. IPv4-mapped IPv6 addresses are checked as IPv4.
func Blocked(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return ip.IsUnspecified()
}

// Transport returns an HTTP transport. With allowPrivate false, connections
// to blocked addresses fail after DNS resolution, which also defeats DNS
// rebinding to an internal address.
func Transport(allowPrivate bool) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || Blocked(ip) {
				return fmt.Errorf("%w: %s", ErrBlockedAddress, host)
			}
			return nil
		},
	}
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
}

// Client returns an HTTP client over Transport that does not follow
// redirects, so a permitted host cannot bounce a request somewhere else.
func Client(allowPrivate bool, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: Transport(allowPrivate),
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
