//go:build !unix

package coding

import "os/exec"

func configureProcessGroup(command *exec.Cmd) {}

func terminateProcessGroup(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}
