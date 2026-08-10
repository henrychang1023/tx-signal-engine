package shioajiproc

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const healthCheckTimeout = 30 * time.Second

// Manager starts, restarts, and stops adapter/shioaji_adapter.py as a
// subprocess, and knows the local URL it serves quotes on once healthy.
type Manager struct {
	dir  string // directory containing shioaji_adapter.py and .venv/
	port int

	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
}

// NewManager builds a Manager for the adapter in dir, whose HTTP server
// will listen on port.
func NewManager(dir string, port int) *Manager {
	return &Manager{dir: dir, port: port}
}

// BaseURL is the local URL the adapter serves quotes on once started.
func (m *Manager) BaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", m.port)
}

// Running reports whether Start has successfully brought up an adapter
// process that hasn't since exited.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// pythonPath returns the venv interpreter path under dir. dir should
// already be absolute — see the comment in Start about relative-Path
// resolution against Dir.
func pythonPath(dir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, ".venv", "Scripts", "python.exe")
	}
	return filepath.Join(dir, ".venv", "bin", "python")
}

// Start (re)launches the adapter with the given credentials and blocks
// until it reports healthy, its process exits early, or healthCheckTimeout
// elapses. Any previously running instance is stopped first, so calling
// Start again is how a credential change gets applied.
func (m *Manager) Start(apiKey, secretKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopLocked()

	// Resolve to an absolute directory before building any paths from it.
	// exec.Cmd resolves a *relative* Path against Dir (not the parent
	// process's CWD) once Dir is set — with m.dir relative (e.g. "adapter")
	// that silently doubled up to "adapter/adapter/...", which Windows then
	// reported as "cannot find the path specified".
	absDir, err := filepath.Abs(m.dir)
	if err != nil {
		return fmt.Errorf("resolve adapter directory %q: %w", m.dir, err)
	}

	pythonExe := pythonPath(absDir)
	script := filepath.Join(absDir, "shioaji_adapter.py")
	if _, err := os.Stat(pythonExe); err != nil {
		return fmt.Errorf("shioaji adapter virtualenv not found at %s — set it up first (see README): %w", pythonExe, err)
	}
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("shioaji_adapter.py not found at %s: %w", script, err)
	}

	cmd := exec.Command(pythonExe, script)
	cmd.Dir = absDir
	cmd.Env = append(os.Environ(),
		"SHIOAJI_API_KEY="+apiKey,
		"SHIOAJI_SECRET_KEY="+secretKey,
		fmt.Sprintf("SHIOAJI_ADAPTER_PORT=%d", m.port),
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("attach stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("attach stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start adapter process: %w", err)
	}
	m.cmd = cmd

	var streamWG sync.WaitGroup
	streamWG.Add(2)
	go func() { defer streamWG.Done(); streamLog(stdout) }()
	go func() { defer streamWG.Done(); streamLog(stderr) }()

	exited := make(chan error, 1)
	go func() {
		streamWG.Wait() // pipes must be fully drained before Wait, per os/exec docs
		exited <- cmd.Wait()
	}()

	if err := waitHealthy(m.BaseURL(), exited); err != nil {
		m.killLocked()
		return err
	}

	m.running = true
	go func() {
		<-exited
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	return nil
}

// Stop kills the running adapter process, if any.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Manager) stopLocked() {
	m.killLocked()
	m.running = false
}

func (m *Manager) killLocked() {
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
	m.cmd = nil
}

func waitHealthy(baseURL string, exited <-chan error) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.After(healthCheckTimeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-exited:
			return fmt.Errorf("adapter process exited before becoming healthy: %v", err)
		case <-deadline:
			return fmt.Errorf("timed out after %s waiting for adapter to become healthy", healthCheckTimeout)
		case <-ticker.C:
			resp, err := client.Get(baseURL + "/health")
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

// streamLog copies the adapter's output line by line into Go's own log,
// so login/subscribe failures show up in the same console the user is
// already watching instead of being silently swallowed.
func streamLog(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		log.Printf("[shioaji-adapter] %s", scanner.Text())
	}
}
