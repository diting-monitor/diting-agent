package version

// Current defines the release version of diting-agent.
// It can be overridden at build time via -ldflags:
//
//	go build -ldflags="-X 'github.com/diting-monitor/diting-agent/internal/version.Current=v1.2.3'"
var Current = "v0.1.0"
