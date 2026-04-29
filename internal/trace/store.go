package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteTrace writes a trace to the given path atomically.
func WriteTrace(path string, t *Trace) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trace: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "aperio-trace-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// ReadTrace reads a trace from the given path.
func ReadTrace(path string) (*Trace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trace file: %w", err)
	}

	var t Trace
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal trace: %w", err)
	}

	return &t, nil
}
