package router

import "toron/pkg/httpparser"

// HasExplicitPort exports hasExplicitPort for tests in router_test package.
func HasExplicitPort(host string) bool {
	return hasExplicitPort(host)
}

// ExtractFullHostPort exports extractFullHostPort for tests in router_test package.
func ExtractFullHostPort(req *httpparser.Request) string {
	return extractFullHostPort(req)
}

// ExtractHost exports extractHost for tests in router_test package.
func ExtractHost(req *httpparser.Request) string {
	return extractHost(req)
}

// ExtractCacheHostPort exports extractCacheHostPort for tests in router_test package.
func ExtractCacheHostPort(req *httpparser.Request) string {
	return extractCacheHostPort(req)
}
