package transfer

import (
	"context"
	"errors"
	"fmt"
)

// Run performs OSS uploads followed by the SSH precommands/transfer/commands flow.
func Run(ctx context.Context, args Options) error {
	var failures []error
	if args.OSS != "" {
		if err := handleOSS(ctx, args); err != nil {
			failures = append(failures, fmt.Errorf("OSS transfer failed: %w", err))
		}
	}
	if args.Destination != "" && ctx.Err() == nil {
		if err := handleSSH(ctx, args); err != nil {
			failures = append(failures, fmt.Errorf("SSH transfer failed: %w", err))
		}
	}
	if err := ctx.Err(); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}
