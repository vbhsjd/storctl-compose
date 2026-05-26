package assets

import _ "embed"

// StorctlLinuxARM64 is replaced by the release workflow before building.
//
//go:embed storctl-linux-arm64
var StorctlLinuxARM64 []byte

func HasEmbeddedStorctl() bool {
	return len(StorctlLinuxARM64) > 4 && string(StorctlLinuxARM64)[:4] == "\x7fELF"
}
