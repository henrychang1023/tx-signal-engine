package shioajiproc

import (
	"os"
	"path/filepath"
)

// LocateAdapterDir finds the "adapter" directory containing
// shioaji_adapter.py: first relative to the current working directory
// (matches `go run ./cmd/server` invoked from the repo root during
// development), then relative to exeDir (matches a distributed build where
// the adapter folder ships alongside the executable). Returns "adapter"
// (the dev-mode relative path) if neither is found, so callers get a
// consistent, traceable path in error messages rather than an empty string.
func LocateAdapterDir(exeDir string) string {
	candidates := []string{"adapter", filepath.Join(exeDir, "adapter")}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "shioaji_adapter.py")); err == nil {
			return dir
		}
	}
	return "adapter"
}
