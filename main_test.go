package main

import (
	"errors"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Provide required token, then verify defaults for other fields.
	cfg, err := loadConfig([]string{"-t", "my-test-token"})
	if err != nil {
		t.Fatalf("failed to load default config: %v", err)
	}

	if cfg.ServerURL != DefaultServer {
		t.Errorf("expected default ServerURL %q, got %q", DefaultServer, cfg.ServerURL)
	}
	if cfg.Token != "my-test-token" {
		t.Errorf("expected Token %q, got %q", "my-test-token", cfg.Token)
	}
}

func TestLoadConfig_MissingToken(t *testing.T) {
	// Missing token must be intercepted with an error immediately.
	_, err := loadConfig([]string{})
	if err == nil {
		t.Error("expected error when token is omitted, but got nil")
	}
}

func TestLoadConfig_Flags(t *testing.T) {
	args := []string{
		"-s", "wss://remote-server:9000/api/v1/ws/rpc",
		"-t", "test-token-xyz",
	}

	cfg, err := loadConfig(args)
	if err != nil {
		t.Fatalf("failed to parse CLI flags: %v", err)
	}

	if cfg.ServerURL != "wss://remote-server:9000/api/v1/ws/rpc" {
		t.Errorf("ServerURL override failed: %v", cfg.ServerURL)
	}
	if cfg.Token != "test-token-xyz" {
		t.Errorf("Token override failed: %v", cfg.Token)
	}
}

func TestLoadConfig_Env(t *testing.T) {
	t.Setenv("DITING_SERVER_URL", "ws://env-host:8080/api/v1/ws/rpc")
	t.Setenv("DITING_TOKEN", "env-token-456")

	cfg, err := loadConfig([]string{})
	if err != nil {
		t.Fatalf("failed to parse environment variables: %v", err)
	}

	if cfg.ServerURL != "ws://env-host:8080/api/v1/ws/rpc" {
		t.Errorf("env ServerURL override failed: %v", cfg.ServerURL)
	}
	if cfg.Token != "env-token-456" {
		t.Errorf("env Token override failed: %v", cfg.Token)
	}
}

func TestLoadConfig_FlagOverridesEnv(t *testing.T) {
	t.Setenv("DITING_SERVER_URL", "ws://from-env:8080/api/v1/ws/rpc")
	t.Setenv("DITING_TOKEN", "env-token")

	args := []string{"-server", "ws://from-flag:8080/api/v1/ws/rpc", "-token", "flag-token"}
	cfg, err := loadConfig(args)
	if err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if cfg.ServerURL != "ws://from-flag:8080/api/v1/ws/rpc" {
		t.Errorf("CLI flag should take precedence over env, got: %v", cfg.ServerURL)
	}
	if cfg.Token != "flag-token" {
		t.Errorf("CLI flag token should take precedence over env, got: %v", cfg.Token)
	}
}

func TestLoadConfig_Validation(t *testing.T) {
	// Invalid protocol test
	_, err := loadConfig([]string{"-t", "valid-token", "-s", "http://localhost:8080"})
	if err == nil {
		t.Error("URLs starting with http:// should fail validation")
	}
}

func TestLoadConfig_Version(t *testing.T) {
	_, err := loadConfig([]string{"-v"})
	if err == nil || !errors.Is(err, ErrVersion) {
		t.Errorf("expected ErrVersion, got: %v", err)
	}

	_, err = loadConfig([]string{"--version"})
	if err == nil || !errors.Is(err, ErrVersion) {
		t.Errorf("expected ErrVersion, got: %v", err)
	}
}
