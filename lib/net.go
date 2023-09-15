package lib

import (
	"fmt"
	"net"
	"net/url"
)

// ResolveDomain takes a domain name and returns its IP addresses.
func ResolveDomain(domain string) ([]net.IP, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return nil, err
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no IPs found for domain %s", domain)
	}

	return ips, nil
}

// GetIPFromURL takes a URL string, parses it to extract the host,
// and then resolves the host to IP addresses.
func GetIPFromURL(urlStr string) ([]net.IP, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	host := parsedURL.Hostname()

	parsedIP := net.ParseIP(host)
	if parsedIP != nil {
		return []net.IP{parsedIP}, nil
	}

	ips, err := ResolveDomain(host)
	if err != nil {
		return nil, err
	}

	return ips, nil
}
