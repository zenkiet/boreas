package docker

import (
	"context"
	"fmt"
	"net"
	"time"
)

type TCPReadyChecker struct{ DialTimeout time.Duration }

func (c TCPReadyChecker) Ready(ctx context.Context, address string) error {
	dialer := net.Dialer{Timeout: c.DialTimeout}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("dial readiness endpoint %s: %w", address, err)
	}
	return connection.Close()
}
