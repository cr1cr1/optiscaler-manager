//go:build windows

package discovery

import "golang.org/x/sys/windows/registry"

// windowsRegistry is the production registryReader backed by the real
// Windows registry (HKLM root).
type windowsRegistry struct{}

func (windowsRegistry) Subkeys(path string) ([]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer func() { _ = k.Close() }()
	return k.ReadSubKeyNames(-1)
}

func (windowsRegistry) ReadString(path, name string) (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue(name)
	return v, err
}
