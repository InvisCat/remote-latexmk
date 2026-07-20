// Package serverurl normalizes remote-latexmk HTTP endpoints.
package serverurl

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const DefaultHTTPPort = "8080"

// Normalize accepts an absolute HTTP(S) URL or a host shorthand. Host
// shorthands and explicit HTTP URLs without a port use remote-latexmk's native
// server default, http://HOST:8080. HTTPS keeps its standard default port.
func Normalize(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("server address is required")
	}
	if strings.ContainsAny(value, "\x00\r\n\t") {
		return "", errors.New("server address must not contain control characters")
	}
	if !strings.Contains(value, "://") {
		if net.ParseIP(value) != nil && strings.Contains(value, ":") {
			value = "[" + value + "]"
		} else if strings.Count(value, ":") > 1 && !strings.HasPrefix(value, "[") {
			return "", errors.New("IPv6 server addresses must use brackets")
		}
		value = "http://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid server address: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("server address must use http or https")
	}
	if parsed.Opaque != "" || parsed.Host == "" || parsed.Hostname() == "" {
		return "", errors.New("server address must include a host")
	}
	if strings.Count(parsed.Host, ":") > 1 && !strings.HasPrefix(parsed.Host, "[") {
		return "", errors.New("IPv6 server addresses must use brackets")
	}
	if parsed.User != nil {
		return "", errors.New("server address must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("server address must not contain a query or fragment")
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return "", errors.New("server address contains an empty port")
	}

	port := parsed.Port()
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return "", fmt.Errorf("server address contains invalid port %q", port)
		}
	} else if parsed.Scheme == "http" {
		parsed.Host = net.JoinHostPort(parsed.Hostname(), DefaultHTTPPort)
	}

	return strings.TrimRight(parsed.String(), "/"), nil
}
