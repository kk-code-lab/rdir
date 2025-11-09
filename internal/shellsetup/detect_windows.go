//go:build windows

package shellsetup

import (
	"os"
	"path"
	"strings"

	"golang.org/x/sys/windows"
)

func DetectParentShellName() string {
	ppid := os.Getppid()
	if ppid <= 0 {
		return ""
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(ppid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)

	buffer := make([]uint16, 512)
	size := uint32(len(buffer))

	for {
		err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size)
		if err == nil {
			break
		}
		if err == windows.ERROR_INSUFFICIENT_BUFFER {
			buffer = make([]uint16, len(buffer)*2)
			size = uint32(len(buffer))
			continue
		}
		return ""
	}

	exePath := windows.UTF16ToString(buffer[:size])
	if exePath == "" {
		return ""
	}

	exePath = strings.ReplaceAll(exePath, "\\", "/")
	name := path.Base(exePath)
	name = strings.TrimSuffix(strings.ToLower(name), ".exe")

	switch name {
	case "pwsh", "powershell":
		return "pwsh"
	case "cmd":
		return "cmd"
	case "bash":
		return "bash"
	default:
		return name
	}
}
