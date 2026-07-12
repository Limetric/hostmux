//go:build darwin || freebsd

package sockserver

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// peerAllowed reports whether the connection's peer belongs to the daemon's
// own user. On macOS/FreeBSD it reads the peer euid via LOCAL_PEERCRED. If the
// peer credentials cannot be determined the connection is allowed (the
// socket's file permissions still gate access), so a failure here never locks
// out the legitimate owner.
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
		cred, cerr := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if cerr == nil {
			peerUID = cred.Uid
			haveCred = true
		}
	})
	if !haveCred {
		return true
	}
	return int(peerUID) == os.Getuid()
}
