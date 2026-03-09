package platform

import "runtime"

type SupportInfo struct {
	OS           string
	Arch         string
	HasCgroups   bool
	HasJobObj    bool
	HasSetRlimit bool
}

func GetCapabilities() SupportInfo {
	info := SupportInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	switch info.OS {
	case "linux":
		info.HasCgroups = true
	case "windows":
		info.HasJobObj = true
	case "darwin", "freebsd":
		info.HasSetRlimit = true
	}

	return info
}
