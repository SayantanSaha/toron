package listener

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/SayantanSaha/toron/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPListener_StartAndStop(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	cfg := config.ServerConfig{
		Address:  ":8081",
		UseTLS:   false,
		CertFile: "",
		KeyFile:  "",
	}

	l := NewHTTPListener(cfg, handler)

	// Start the listener in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- l.Start()
	}()

	// Allow time for the server to start
	time.Sleep(200 * time.Millisecond)

	resp, err := http.Get("http://localhost:8081")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Stop the server
	err = l.Stop(context.Background())
	require.NoError(t, err)

	// Wait for goroutine to finish
	select {
	case err := <-done:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("server shutdown timed out")
	}
}
