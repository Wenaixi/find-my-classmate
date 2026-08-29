package main

import (
	"net/http"
	"testing"
	"time"
)

func TestBuildServerConfiguresTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	server := buildServer(":0", handler)

	if server.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %v，期望 10s", server.ReadTimeout)
	}
	if server.WriteTimeout != 15*time.Second {
		t.Errorf("WriteTimeout = %v，期望 15s", server.WriteTimeout)
	}
	if server.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v，期望 60s", server.IdleTimeout)
	}
	if server.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v，期望 5s", server.ReadHeaderTimeout)
	}
}
