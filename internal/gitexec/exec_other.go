//go:build !unix

package gitexec

import "os/exec"

// git cannot be started without a terminal here: u/U stay hidden.
const supported = false

func detach(*exec.Cmd) {}
