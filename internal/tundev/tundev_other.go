//go:build !linux

package tundev

import "errors"

// errUnsupported is returned by Start on platforms without a TUN implementation,
// letting the caller fall back to SOCKS-only cleanly.
var errUnsupported = errors.New("TUN mode is only supported on Linux; SOCKS remains available")

// Instance is a no-op placeholder on non-Linux platforms.
type Instance struct{}

// Name returns the empty string on unsupported platforms.
func (in *Instance) Name() string { return "" }

// Start reports that TUN mode is unavailable on this OS.
func Start(_ Node) (*Instance, error) { return nil, errUnsupported }

// Close is a no-op on unsupported platforms.
func (in *Instance) Close() error { return nil }
