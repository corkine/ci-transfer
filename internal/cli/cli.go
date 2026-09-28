package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/corkine/ci-transfer/internal/transfer"
)

// Options combines transfer settings with CLI-only display flags.
type Options struct {
	transfer.Options
	Help, Version bool
}

const Usage = `ci-transfer — upload files to OSS or SSH and execute remote commands

Usage: ci-transfer [options]
  -s, --source PATH...         Files/directories (repeatable; optional)
  -d, --destination VALUE      user:password@host:/path, or Base64
      --oss-destination VALUE  OSS JSON configuration, or Base64
      --precommands CMD...    Commands before SSH transfer (repeatable)
  -c, --commands CMD...        Commands after SSH transfer (repeatable)
      --port PORT             SSH port (default 22)
      --retries N             Extra attempts for transient failures (default 3; 0 disables)
      --retry-delay DURATION  Initial backoff (default 2s; doubles up to 30s)
      --timeout DURATION      Timeout per file upload attempt (default 10m)
      --connect-timeout D     Connection/SSH handshake timeout (default 30s)
      --command-timeout D     Timeout per command (default 0: unlimited; never retried)
      --known-hosts PATH      SSH known_hosts file (optional, legacy default: no verification)
  -h, --help                  Show help
  -V, --version               Show version

Durations use Go notation, e.g. 30s, 10m. SSH commands are never retried.
`

// Parse accepts the legacy Rust CLI's repeated and multi-value options.
func Parse(argv []string) (Options, error) {
	a := Options{Options: transfer.Options{Port: 22, Retries: 3, RetryDelay: 2 * time.Second, Timeout: 10 * time.Minute, ConnectTimeout: 30 * time.Second}}
	for i := 0; i < len(argv); i++ {
		name, inline, hasInline := strings.Cut(argv[i], "=")
		var list *[]string
		switch name {
		case "-h", "--help":
			a.Help = true
			continue
		case "-V", "--version":
			a.Version = true
			continue
		case "-s", "--source":
			list = &a.Sources
		case "--precommands":
			list = &a.Precommands
		case "-c", "--commands":
			list = &a.Commands
		case "-d", "--destination", "--oss-destination", "--port", "--retries", "--retry-delay", "--timeout", "--connect-timeout", "--command-timeout", "--known-hosts":
		default:
			return a, fmt.Errorf("unknown option %q (see --help)", name)
		}
		if list != nil {
			if hasInline {
				*list = append(*list, inline)
				continue
			}
			start := i
			for i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
				i++
				*list = append(*list, argv[i])
			}
			if i == start {
				return a, fmt.Errorf("%s requires a value", name)
			}
			continue
		}
		value := inline
		if !hasInline {
			if i+1 >= len(argv) {
				return a, fmt.Errorf("%s requires a value", name)
			}
			i++
			value = argv[i]
		}
		var err error
		switch name {
		case "-d", "--destination":
			a.Destination = value
		case "--oss-destination":
			a.OSS = value
		case "--known-hosts":
			a.KnownHosts = value
		case "--port":
			a.Port, err = strconv.Atoi(value)
		case "--retries":
			a.Retries, err = strconv.Atoi(value)
		case "--retry-delay":
			a.RetryDelay, err = time.ParseDuration(value)
		case "--timeout":
			a.Timeout, err = time.ParseDuration(value)
		case "--connect-timeout":
			a.ConnectTimeout, err = time.ParseDuration(value)
		case "--command-timeout":
			a.CommandTimeout, err = time.ParseDuration(value)
		}
		if err != nil {
			return a, fmt.Errorf("invalid value for %s", name)
		}
	}
	if a.Help || a.Version {
		return a, nil
	}
	if a.Port < 1 || a.Port > 65535 {
		return a, fmt.Errorf("--port must be between 1 and 65535")
	}
	if a.Retries < 0 || a.Retries > 100 {
		return a, fmt.Errorf("--retries must be between 0 and 100")
	}
	if a.RetryDelay <= 0 || a.Timeout <= 0 || a.ConnectTimeout <= 0 || a.CommandTimeout < 0 {
		return a, fmt.Errorf("timeouts and retry delay must be positive; command timeout may be 0")
	}
	if a.OSS == "" && a.Destination == "" {
		return a, fmt.Errorf("--destination or --oss-destination is required")
	}
	if a.Destination == "" && (len(a.Commands) > 0 || len(a.Precommands) > 0) {
		return a, fmt.Errorf("SSH commands require --destination")
	}
	return a, nil
}
