package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SayantanSaha/toron/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_MatchesRoute(t *testing.T) {
	r := NewRouter([]config.Route{
		{Path: "/api", Backend: "http://localhost:9000"},
		{Path: "/blog", Backend: "http://localhost:9001"},
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Contains(t, string(body), "proxy error")
}

func TestRouter_NoMatch(t *testing.T) {
	r := NewRouter([]config.Route{
		{Path: "/api", Backend: "http://localhost:9000"},
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/unknown")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Contains(t, string(body), "route not found")
}
