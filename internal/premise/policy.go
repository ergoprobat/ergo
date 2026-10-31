package premise

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var errURL = errors.New("url")

// parseProbeURL accepts an https URL with a DNS name, no userinfo, no fragment,
// and a port that is absent or 443. A query string is allowed.
func parseProbeURL(raw string) (*url.URL, error) {
	if strings.Contains(raw, "#") {
		return nil, errURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil {
		return nil, errURL
	}
	host := parsed.Hostname()
	if host == "" || net.ParseIP(host) != nil {
		return nil, errURL
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return nil, errURL
	}
	return parsed, nil
}

// allowedAddr reports whether ip is a public unicast address.
// Go stores IPv4 as an IPv4-mapped IPv6 address, so a mapped address is judged
// by its IPv4 rules and dialed as IPv4. A mapped private address is refused.
func allowedAddr(ip net.IP) bool {
	if len(ip) == 0 {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		return allowedV4(v4)
	}
	return allowedV6(ip)
}

func allowedV4(ip net.IP) bool {
	if !ip.IsGlobalUnicast() {
		return false
	}
	if ip[0] == 0 || ip[0] == 10 {
		return false
	}
	if ip[0] == 172 && ip[1]&0xf0 == 16 {
		return false
	}
	if ip[0] == 192 && ip[1] == 168 {
		return false
	}
	if ip[0] == 100 && ip[1]&0xc0 == 0x40 {
		return false
	}
	return true
}

func allowedV6(ip net.IP) bool {
	if len(ip) != net.IPv6len || !ip.IsGlobalUnicast() {
		return false
	}
	// fc00::/7 is unique local, not a public address.
	if ip[0]&0xfe == 0xfc {
		return false
	}
	return true
}
