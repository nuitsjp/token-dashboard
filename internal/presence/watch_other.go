//go:build !windows

package presence

import "context"

// Watch does nothing off Windows: the user is never reported away.
func Watch(ctx context.Context, c *Conditions, idleAfter func() int) {}
