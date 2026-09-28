package transfer

import "time"

// Options controls transfer behavior independently of CLI parsing.
type Options struct {
	Sources, Precommands, Commands                      []string
	Destination, OSS, KnownHosts                        string
	Port, Retries                                       int
	RetryDelay, Timeout, ConnectTimeout, CommandTimeout time.Duration
}
