// Package profile writes the curated safe-defaults OptiScaler.ini applied at
// install time. Editing beyond this is deliberately out of scope: users open
// the file in their system editor (docs/scope.md).
package profile

import "io"

// defaults is the curated minimal profile: let OptiScaler pick the best
// upscaler and frame generator per game and API.
const defaults = `[Upscalers]
Dx11Upscaler=auto
Dx12Upscaler=auto
VulkanUpscaler=auto

[FrameGen]
FGType=auto
`

// WriteDefaultINI writes the curated safe-defaults OptiScaler.ini.
func WriteDefaultINI(w io.Writer) error {
	_, err := io.WriteString(w, defaults)
	return err
}
