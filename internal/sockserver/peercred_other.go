//go:build !linux && !darwin && !freebsd

package sockserver

import "net"

// peerAllowed is a no-op on platforms without a supported peer-credential API
// (notably Windows, OpenBSD, and NetBSD). Access to the control socket is
// gated by the socket's file-system permissions on those platforms.
func peerAllowed(c net.Conn) bool { return true }
