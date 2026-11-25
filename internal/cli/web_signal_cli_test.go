//go:build linux

package cli

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/testenv"
)

// TestWebShutdown runs the real Main entry point in a child process. The
// global probe handler used to kill magpie web with the signal's status;
// the web command must take SIGINT and SIGTERM back, drain its servers, and
// return success. SIGHUP remains the global handler's, and page Quit still
// exits successfully.
func TestWebShutdown(t *testing.T) {
	for _, mode := range []string{"SIGTERM", "SIGINT", "SIGHUP", "quit"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			config := filepath.Join(home, ".config", "magpie")
			if err := os.MkdirAll(config, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config, "settings.json"), []byte(`{"noAutoUpdate":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(home, "web.log")
			logFile, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer logFile.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, self, "-test.run=^$")
			cmd.Dir = home
			cmd.Env = append(os.Environ(),
				"MAGPIE_CLI_TEST_WEB_SHUTDOWN=1",
				"HOME="+home,
				"USERPROFILE="+home,
				"XDG_CONFIG_HOME="+filepath.Dir(config),
				"XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
				"XDG_DATA_HOME="+filepath.Join(home, ".local", "share"),
				"XDG_STATE_HOME="+filepath.Join(home, ".local", "state"),
				"MAGPIE_ADDR=127.0.0.1:0",
				"MAGPIE_WEB_KEY=web-shutdown-test-key",
				"NO_COLOR=1",
				"HTTP_PROXY=http://127.0.0.1:1",
				"HTTPS_PROXY=http://127.0.0.1:1",
				"NO_PROXY=127.0.0.1",
				testenv.Marker+"="+os.Getenv(testenv.Marker),
			)
			cmd.Stdout, cmd.Stderr = logFile, logFile
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			waited := false
			defer func() {
				if waited {
					return
				}
				cancel()
				_ = cmd.Process.Kill()
				<-done
			}()

			link := regexp.MustCompile(`http://127\.0\.0\.1:[0-9]+/\?k=web-shutdown-test-key`)
			var url string
			for url == "" {
				b, _ := os.ReadFile(logPath)
				url = link.FindString(string(b))
				select {
				case err := <-done:
					waited = true
					t.Fatalf("web exited before ready: %v\n%s", err, b)
				case <-time.After(20 * time.Millisecond):
				}
			}

			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("web readiness: HTTP %d", resp.StatusCode)
			}

			if mode == "quit" {
				quit := resp.Request.URL.Scheme + "://" + resp.Request.URL.Host + "/api/window/quit"
				resp, err = client.Post(quit, "application/json", nil)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusNoContent {
					t.Fatalf("quit: HTTP %d", resp.StatusCode)
				}
			} else {
				sig := map[string]os.Signal{
					"SIGTERM": syscall.SIGTERM,
					"SIGINT":  syscall.SIGINT,
					"SIGHUP":  syscall.SIGHUP,
				}[mode]
				if err := cmd.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
			}

			err = <-done
			waited = true
			if ctx.Err() != nil {
				b, _ := os.ReadFile(logPath)
				t.Fatalf("web did not stop: %v\n%s", ctx.Err(), b)
			}
			if mode == "SIGHUP" {
				if err == nil {
					t.Fatal("hangup should still terminate the process")
				}
			} else if err != nil {
				b, _ := os.ReadFile(logPath)
				t.Fatalf("%s should exit successfully: %v\n%s", mode, err, b)
			}
		})
	}
}
