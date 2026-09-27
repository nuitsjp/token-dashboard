//go:build !windows

package turzx

// List returns no devices outside Windows.
func List() ([]Device, error) { return []Device{}, nil }
