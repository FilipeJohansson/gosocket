// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package utils

import (
	"net"
	"net/http"
)

// GetIPFromRequest returns the client's IP address from the given http request.
// If the request contains a valid X-Real-IP or X-Forwarded-For header, it returns the IP
// address from the header. Otherwise, it returns the IP address from the remote address.
// The IP address is returned as a string in the format "192.0.2.1".
func GetIPFromRequest(r *http.Request) string {
	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		ip, _, _ := net.SplitHostPort(ip)
		return ip
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// ExtractIP returns the IP address from the given net.Addr.
func ExtractIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

// ExtractHeaders extracts relevant headers from the given http request.
// It returns a map of header name to header value. Only the following
// headers are considered: Authorization, X-Forwarded-For, X-Real-IP,
// Accept, Accept-Language, and Accept-Encoding. If a header is not
// present in the request, it is not included in the returned map.
func ExtractHeaders(r *http.Request, extraHeaders ...string) map[string]string {
	headers := make(map[string]string)

	// common headers that might be useful in handlers
	relevantHeaders := []string{ // TODO: make configurable
		"Authorization",
		"X-Forwarded-For",
		"X-Real-Ip",
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
	}
	relevantHeaders = append(relevantHeaders, extraHeaders...)

	for _, header := range relevantHeaders {
		if value := r.Header.Get(header); value != "" {
			headers[header] = value
		}
	}

	return headers
}
