package update

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func launchInstaller(path string) error {
	cmd := exec.Command(path, "/WAITPID="+strconv.Itoa(os.Getpid()))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start setup; SADPTools will stay open: %w", err)
	}
	// A successfully created child can outlive this process. No shell, forced
	// termination, elevation request, silent license acceptance or retries.
	_ = cmd.Process.Release()
	return nil
}
