//go:build windows

package main

// Windows has no O_NOFOLLOW, and O_NONBLOCK carries no meaning for
// CreateFile, so the capture file opens with no extra flags; it is still
// verified to be a regular file after opening.
const trafficCaptureOpenFlags = 0
