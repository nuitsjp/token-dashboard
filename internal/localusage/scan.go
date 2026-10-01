package localusage

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// scan is what the reader keeps of `clients --json`, so that it runs only for a new tool.
type scan struct {
	// Paths are the scan locations to watch.
	Paths []string `json:"paths"`
	// Clients are the tools that graph showed when the paths were saved.
	Clients []string `json:"clients"`
}

func loadScan(path string) (scan, error) {
	var s scan
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

// saveScan writes s to a temporary file next to path and then replaces path with it.
func saveScan(path string, s scan) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
