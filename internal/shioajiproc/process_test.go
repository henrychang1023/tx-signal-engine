package shioajiproc

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Config{APIKey: "key-123", SecretKey: "secret-456"}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestConfigLoad_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	if _, err := Load(path); err == nil {
		t.Fatal("expected an error loading a missing config file")
	}
}

func TestPythonPath(t *testing.T) {
	got := pythonPath("adapter")

	var want string
	if runtime.GOOS == "windows" {
		want = filepath.Join("adapter", ".venv", "Scripts", "python.exe")
	} else {
		want = filepath.Join("adapter", ".venv", "bin", "python")
	}
	if got != want {
		t.Fatalf("pythonPath(%q) = %q, want %q", "adapter", got, want)
	}
}

func TestManager_BaseURL(t *testing.T) {
	m := NewManager("adapter", 9999)
	if got, want := m.BaseURL(), "http://127.0.0.1:9999"; got != want {
		t.Fatalf("BaseURL() = %q, want %q", got, want)
	}
}

func TestLocateAdapterDir_PrefersCWDRelative(t *testing.T) {
	root := t.TempDir()
	cwdAdapter := filepath.Join(root, "cwd-side", "adapter")
	exeAdapter := filepath.Join(root, "exe-side", "adapter")
	if err := os.MkdirAll(cwdAdapter, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(exeAdapter, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cwdAdapter, "shioaji_adapter.py"), "")
	writeFile(t, filepath.Join(exeAdapter, "shioaji_adapter.py"), "")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(filepath.Join(root, "cwd-side")); err != nil {
		t.Fatal(err)
	}

	got := LocateAdapterDir(filepath.Join(root, "exe-side"))
	if got != "adapter" {
		t.Fatalf("LocateAdapterDir() = %q, want the CWD-relative %q", got, "adapter")
	}
}

func TestLocateAdapterDir_FallsBackToExeDir(t *testing.T) {
	root := t.TempDir()
	exeAdapter := filepath.Join(root, "exe-side", "adapter")
	if err := os.MkdirAll(exeAdapter, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(exeAdapter, "shioaji_adapter.py"), "")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(t.TempDir()); err != nil { // CWD with no adapter/ at all
		t.Fatal(err)
	}

	got := LocateAdapterDir(filepath.Join(root, "exe-side"))
	if want := filepath.Join(root, "exe-side", "adapter"); got != want {
		t.Fatalf("LocateAdapterDir() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
