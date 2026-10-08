package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/diting-monitor/diting-agent/internal/collector"
	"github.com/diting-monitor/diting-agent/internal/version"
	"github.com/diting-monitor/diting-protocol"
	"github.com/gorilla/websocket"
)

// Client manages the full-duplex WebSocket connection to the Diting Server.
type Client struct {
	serverURL string
	token     string
	mu        sync.Mutex
	conn      *websocket.Conn
	backoff   *backoff
}

// New creates a new agent communication client.
func New(serverURL string, token string) *Client {
	return &Client{
		serverURL: serverURL,
		token:     token,
		backoff:   newBackoff(1*time.Second, 30*time.Second, 2.0),
	}
}

// Start launches the main client loop with automatic exponential backoff reconnection.
func (c *Client) Start(ctx context.Context, col *collector.Collector, interval time.Duration) {
	for {
		if ctx.Err() != nil {
			slog.Info("agent client stopped")
			return
		}

		err := c.connectAndLoop(ctx, col, interval)
		if err != nil {
			// If shutdown was requested, exit cleanly without logging a retry warning.
			if ctx.Err() != nil {
				slog.Info("agent client stopped")
				return
			}

			waitDuration := c.backoff.duration()
			slog.Warn("connection error, scheduling reconnect",
				slog.Any("error", err),
				slog.Duration("retry_after", waitDuration.Round(time.Millisecond)),
			)

			timer := time.NewTimer(waitDuration)
			select {
			case <-ctx.Done():
				timer.Stop()
				slog.Info("agent client stopped")
				return
			case <-timer.C:
			}
		}
	}
}

// connectAndLoop executes dial, authentication handshake, and the telemetry reporting loop.
func (c *Client) connectAndLoop(ctx context.Context, col *collector.Collector, interval time.Duration) error {
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	slog.Info("connecting to server", slog.String("url", u.String()))

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 5 * time.Second

	header := http.Header{}
	header.Set("User-Agent", "Diting-Agent/"+version.Current)
	header.Set("Authorization", "Bearer "+c.token)

	conn, resp, err := dialer.DialContext(ctx, u.String(), header)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		if resp != nil {
			return fmt.Errorf("handshake failed (HTTP %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("dial failed: %w", err)
	}
	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		_ = conn.Close()
	}()

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Reset backoff duration upon successful connection.
	c.backoff.reset()
	slog.Info("connected to server", slog.String("server", u.Host))

	// 1. Send static system and hardware metadata upon initial connection (agent.meta).
	metaParams, err := col.CollectMeta()
	if err != nil {
		return fmt.Errorf("collect meta failed: %w", err)
	}

	metaNotice := protocol.NewNotification(protocol.MethodAgentMeta, metaParams)
	if err := c.send(metaNotice); err != nil {
		return fmt.Errorf("send meta failed: %w", err)
	}
	slog.Info("registered node metadata (agent.meta)",
		slog.String("hostname", metaParams.Hostname),
		slog.String("os", metaParams.OS),
		slog.String("arch", metaParams.Arch),
	)

	// 2. Start periodic metrics telemetry ticker (agent.metrics).
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Listen for server disconnects or control frames.
	closeChan := make(chan error, 1)
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				closeChan <- err
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			// Gracefully close WebSocket connection with write lock.
			c.mu.Lock()
			_ = conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Agent Shutdown"))
			c.mu.Unlock()
			return nil
		case err := <-closeChan:
			return fmt.Errorf("connection closed by server: %w", err)
		case <-ticker.C:
			metricsParams, err := col.CollectMetrics()
			if err != nil {
				slog.Warn("metrics collection failed", slog.Any("error", err))
				continue
			}

			metricsNotice := protocol.NewNotification(protocol.MethodAgentMetrics, metricsParams)
			if err := c.send(metricsNotice); err != nil {
				return fmt.Errorf("send metrics failed: %w", err)
			}
			slog.Debug("metrics reported (agent.metrics)",
				slog.Float64("cpu_pct", metricsParams.Metrics.CPUPercent),
				slog.Uint64("mem_mb", metricsParams.Metrics.MemoryUsed/(1024*1024)),
				slog.Uint64("net_in_rate", metricsParams.Metrics.NetInRate),
				slog.Uint64("net_out_rate", metricsParams.Metrics.NetOutRate),
			)
		}
	}
}

// send serializes a message to JSON and writes it as a WebSocket text frame.
func (c *Client) send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return errors.New("connection not established")
	}

	bytes, err := json.Marshal(v)
	if err != nil {
		return err
	}

	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.conn.WriteMessage(websocket.TextMessage, bytes)
}
