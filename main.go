package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/diting-monitor/diting-agent/internal/client"
	"github.com/diting-monitor/diting-agent/internal/collector"
	"github.com/diting-monitor/diting-agent/internal/version"
)

const (
	DefaultServer   = "ws://127.0.0.1:8080/api/v1/ws/rpc"
	DefaultInterval = 2 * time.Second
	MinInterval     = 500 * time.Millisecond
)

// ErrVersion is a sentinel error returned when -v/--version is requested,
// signaling main() to print version information and terminate cleanly.
var ErrVersion = errors.New("version requested")

// Config encapsulates the essential startup and operational configurations for the agent.
type Config struct {
	ServerURL string
	Token     string
	Interval  time.Duration
}

func main() {
	// 1. Load and validate configurations (Priority: Flag > Env > Default).
	cfg, err := loadConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		if errors.Is(err, ErrVersion) {
			fmt.Printf("Diting Agent %s (%s/%s)\n", version.Current, runtime.GOOS, runtime.GOARCH)
			os.Exit(0)
		}
		_, _ = fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize standard structured logger.
	initLogger()

	slog.Info("Starting Diting Agent daemon",
		slog.String("version", version.Current),
		slog.String("server", cfg.ServerURL),
		slog.Duration("interval", cfg.Interval),
	)

	// 3. Graceful shutdown: listen for POSIX signals (SIGINT/SIGTERM) to cancel context.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 4. Initialize core metrics collector facade and WebSocket client.
	col := collector.New()
	wsClient := client.New(cfg.ServerURL, cfg.Token)

	// 5. Start WebSocket connection with exponential backoff and telemetry loop (blocks until shutdown).
	wsClient.Start(ctx, col, cfg.Interval)

	slog.Info("Diting Agent shutdown cleanly")
}

// loadConfig parses CLI flags and environment variables, applying defaults and validations.
func loadConfig(args []string) (*Config, error) {
	fs := flag.NewFlagSet("diting-agent", flag.ContinueOnError)

	var (
		serverFlag   string
		tokenFlag    string
		intervalFlag string
		versionFlag  bool
	)

	fs.StringVar(&serverFlag, "s", "", "Target server WebSocket RPC URL")
	fs.StringVar(&serverFlag, "server", "", "Target server WebSocket RPC URL (long option)")
	fs.StringVar(&tokenFlag, "t", "", "Authentication token for server communication")
	fs.StringVar(&tokenFlag, "token", "", "Authentication token for server communication (long option)")
	fs.StringVar(&intervalFlag, "i", "", "Telemetry reporting interval (e.g. 1s, 2s, 5s)")
	fs.StringVar(&intervalFlag, "interval", "", "Telemetry reporting interval (long option)")
	fs.BoolVar(&versionFlag, "v", false, "Print agent version and exit")
	fs.BoolVar(&versionFlag, "version", false, "Print agent version and exit (long option)")

	fs.Usage = func() {
		out := fs.Output()
		_, _ = fmt.Fprintf(out, "Diting Agent - Lightweight system telemetry daemon (%s)\n\n", version.Current)
		_, _ = fmt.Fprintf(out, "Usage:\n  diting-agent [options]\n\nOptions:\n"+
			"  -s, --server string    Target server WebSocket RPC URL\n"+
			"  -t, --token string     Authentication token for server communication\n"+
			"  -i, --interval string  Telemetry reporting interval (e.g. 1s, 2s, 5s)\n"+
			"  -v, --version          Print agent version and exit\n\n"+
			"Environment Variables (Priority: CLI Flag > Environment > Default):\n"+
			"  DITING_SERVER_URL       Target WebSocket RPC URL (default: %s)\n"+
			"  DITING_TOKEN            Authentication token (required)\n"+
			"  DITING_REPORT_INTERVAL  Reporting interval (default: %v)\n\n", DefaultServer, DefaultInterval)
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Handle version query request.
	if versionFlag {
		return nil, ErrVersion
	}

	// Resolve configuration values by priority: Flag > Env > Default.
	serverURL := strings.TrimSpace(serverFlag)
	if serverURL == "" {
		if env := strings.TrimSpace(os.Getenv("DITING_SERVER_URL")); env != "" {
			serverURL = env
		} else {
			serverURL = DefaultServer
		}
	}

	token := strings.TrimSpace(tokenFlag)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("DITING_TOKEN"))
	}

	interval := DefaultInterval
	rawInterval := strings.TrimSpace(intervalFlag)
	if rawInterval == "" {
		rawInterval = strings.TrimSpace(os.Getenv("DITING_REPORT_INTERVAL"))
	}
	if rawInterval != "" {
		parsed, err := time.ParseDuration(rawInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid interval format %q: %w", rawInterval, err)
		}
		interval = parsed
	}

	// Defensive validations.
	if token == "" {
		return nil, errors.New("missing authentication token: please specify via -t/--token or DITING_TOKEN environment variable")
	}
	if !strings.HasPrefix(serverURL, "ws://") && !strings.HasPrefix(serverURL, "wss://") {
		return nil, fmt.Errorf("server URL must start with ws:// or wss://: %q", serverURL)
	}
	if interval < MinInterval {
		return nil, fmt.Errorf("reporting interval cannot be less than %v (got %v)", MinInterval, interval)
	}

	return &Config{
		ServerURL: serverURL,
		Token:     token,
		Interval:  interval,
	}, nil
}

// initLogger configures standard structured logging using log/slog.
func initLogger() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
}
