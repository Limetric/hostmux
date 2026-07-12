//go:build linux

package sockserver

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// peerAllowed reports whether the connection's peer belongs to the daemon's
// own user. On Linux it reads SO_PEERCRED. If the peer credentials cannot be
// determined the connection is allowed (the socket's file permissions still
// gate access), so a failure here never locks out the legitimate owner.
func peerAllowed(c net.Conn) bool {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return true
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return true
	}
	var (
		peerUID  uint32
		haveCred bool
	)
	_ = raw.Control(func(fd uintptr) {
		ucred, cerr := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if cerr == nil {
			peerUID = ucred.Uid
			haveCred = true
		}
	})
	if !haveCred {
		return true
	}
	return int(peerUID) == os.Getuid()
}
