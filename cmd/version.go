package optiscalermanager

import (
	"fmt"
)

// VersionCmd shows version information
type VersionCmd struct{}

// Run prints the application name and version.
func (v *VersionCmd) Run(version string) error {
	fmt.Printf("%s %s\n", appName, version)
	return nil
}
