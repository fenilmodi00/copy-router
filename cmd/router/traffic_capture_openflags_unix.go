//go:build !windows

package main

import "syscall"

// trafficCaptureOpenFlags adds the symlink-attack guard to the capture file
// open: O_NOFOLLOW refuses to follow a symlink at the capture path, and
// O_NONBLOCK keeps the open from blocking when a FIFO is planted there.
const trafficCaptureOpenFlags = syscall.O_NOFOLLOW | syscall.O_NONBLOCK
