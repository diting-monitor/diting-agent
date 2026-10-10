package client

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diting-monitor/diting-agent/internal/collector"
	"github.com/diting-monitor/diting-agent/internal/version"
	"github.com/diting-monitor/diting-protocol"
	"github.com/gorilla/websocket"
)

func TestMain(m *testing.M) {
	// Silence slog logs during unit tests to eliminate noisy WARN/INFO messages in test output.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// TestClient_Lifecycle verifies connection handshake, Bearer token auth,
// metadata registration (agent.meta), periodic metrics reporting (agent.metrics),
// and graceful WebSocket shutdown.
func TestClient_Lifecycle(t *testing.T) {
	const testToken = "test-secret-token"
	metaReceived := make(chan struct{})
	metricsReceived := make(chan struct{})

	// Mock WebSocket server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Verify Authorization Header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+testToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 2. Verify User-Agent Header.
		expectedUA := "Diting-Agent/" + version.Current
		if r.Header.Get("User-Agent") != expectedUA {
			http.Error(w, "Invalid User-Agent", http.StatusBadRequest)
			return
		}

		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("WebSocket upgrade failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var frame struct {
				JSONRPC string `json:"jsonrpc"`
				Method  string `json:"method"`
			}
			if err := json.Unmarshal(msg, &frame); err != nil {
				continue
			}

			switch frame.Method {
			case protocol.MethodAgentMeta:
				select {
				case <-metaReceived:
				default:
					close(metaReceived)
				}
			case protocol.MethodAgentMetrics:
				select {
				case <-metricsReceived:
				default:
					close(metricsReceived)
				}
			}
		}
	}))
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	client := New(wsURL, testToken)
	client.interval = 50 * time.Millisecond
	col := collector.New()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	clientDone := make(chan struct{})
	go func() {
		client.Start(ctx, col)
		close(clientDone)
	}()

	// Wait for meta to be received.
	select {
	case <-metaReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for agent.meta notification")
	}

	// Wait for metrics to be received.
	select {
	case <-metricsReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for agent.metrics notification")
	}

	// Signal graceful shutdown.
	cancel()

	select {
	case <-clientDone:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not stop cleanly on context cancellation")
	}
}

// TestClient_SendWithoutConnection verifies that calling send before connection returns an error.
func TestClient_SendWithoutConnection(t *testing.T) {
	c := New("ws://127.0.0.1:1", "token")
	err := c.send("payload")
	if err == nil || err.Error() != "connection not established" {
		t.Fatalf("expected 'connection not established' error, got: %v", err)
	}
}

// TestClient_HandshakeFailure_Retry verifies that the client retries on 401 handshake failure
// and successfully connects once valid authentication is provided.
func TestClient_HandshakeFailure_Retry(t *testing.T) {
	var attempt atomic.Int32
	successCh := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempt.Add(1)
		if n == 1 {
			// First attempt: reject with 401
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Subsequent attempt: upgrade successfully
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		select {
		case <-successCh:
		default:
			close(successCh)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := New(wsURL, "token")
	// Set fast backoff for testing to keep unit tests fast:
	client.backoff = newBackoff(10*time.Millisecond, 50*time.Millisecond, 2.0)

	col := collector.New()
	go client.Start(t.Context(), col)

	select {
	case <-successCh:
	case <-time.After(3 * time.Second):
		t.Fatal("client failed to reconnect after handshake failure")
	}
}
