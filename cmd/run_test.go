//go:build !windows

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func setupRunProject(t *testing.T) string {
	t.Helper()
	return setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
environments:
  dev: {}
  prod: {}
defaults:
  env: dev
`,
		"config/base.yaml": "app:\n  name: base\n",
		"config/prod.yaml": "app:\n  name: production\n",
	})
}

func TestRunRun_FlagParsing(t *testing.T) {
	dir := setupRunProject(t)

	tests := []struct {
		name     string
		args     []string
		contains []string
		excludes []string
	}{
		{
			name:     "equals form of --env",
			args:     []string{"run", "--dry-run", "--all", "--env=prod", "-p", dir, "--", "echo"},
			contains: []string{"APP_NAME=production"},
		},
		{
			name:     "--path long form",
			args:     []string{"run", "--dry-run", "--all", "--path", dir, "--", "echo"},
			contains: []string{"APP_NAME=base"},
		},
		{
			name:     "separator is optional",
			args:     []string{"run", "--dry-run", "--all", "-p", dir, "echo", "hi"},
			contains: []string{"Command: [echo hi]"},
		},
		{
			name:     "flags after the command belong to the child",
			args:     []string{"run", "--dry-run", "--all", "-p", dir, "node", "--env", "prod", "-c", "x.txt"},
			contains: []string{"APP_NAME=base", "Command: [node --env prod -c x.txt]"},
			excludes: []string{"production", "Config file"},
		},
		{
			name:     "separator after the command is passed through",
			args:     []string{"run", "--dry-run", "--all", "-p", dir, "--", "sh", "--", "x"},
			contains: []string{"Command: [sh -- x]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := execTestCmd(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v\noutput: %s", err, output)
			}
			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("expected %q in output, got: %s", want, output)
				}
			}
			for _, unwanted := range tt.excludes {
				if strings.Contains(output, unwanted) {
					t.Errorf("did not expect %q in output, got: %s", unwanted, output)
				}
			}
		})
	}
}

func TestRunRun_FlagErrors(t *testing.T) {
	dir := setupRunProject(t)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown flag", []string{"run", "--dry-run", "-p", dir, "--bogus", "--", "echo"}, "unknown flag: --bogus"},
		{"no command", []string{"run", "-p", dir, "--"}, "no command specified"},
		{"no args", []string{"run"}, "no command specified"},
		{"flag missing value", []string{"run", "-p", dir, "--config-file"}, "flag needs an argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := execTestCmd(tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestRunRun_Help(t *testing.T) {
	output, err := execTestCmd("run", "--help")
	if err != nil {
		t.Fatalf("run --help failed: %v", err)
	}
	for _, want := range []string{"Usage:", "--config-file", "--all"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in help, got: %s", want, output)
		}
	}
}

func TestRunRun_ExitCodePropagates(t *testing.T) {
	dir := setupRunProject(t)

	_, err := execTestCmd("run", "-p", dir, "--", "sh", "-c", "exit 4")

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 4 {
		t.Fatalf("expected ExitError{Code: 4}, got %v", err)
	}
}

func TestExecuteCommand(t *testing.T) {
	t.Run("injected vars reach the child and override the parent env", func(t *testing.T) {
		t.Setenv("COFFER_TEST_VAR", "from-parent")
		out := filepath.Join(t.TempDir(), "out")

		err := executeCommand(
			[]string{"sh", "-c", `printf '%s|%s' "$COFFER_TEST_VAR" "$COFFER_TEST_OTHER" > "$OUT"`},
			map[string]string{"COFFER_TEST_VAR": "from-coffer", "COFFER_TEST_OTHER": "x", "OUT": out},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, _ := os.ReadFile(out)
		if string(got) != "from-coffer|x" {
			t.Errorf("expected child to see from-coffer|x, got %q", got)
		}
	})

	t.Run("success returns nil", func(t *testing.T) {
		if err := executeCommand([]string{"true"}, nil); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("non-zero exit", func(t *testing.T) {
		err := executeCommand([]string{"sh", "-c", "exit 3"}, nil)
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 3 {
			t.Fatalf("expected ExitError{Code: 3}, got %v", err)
		}
	})

	t.Run("child killed by signal exits 128+signal", func(t *testing.T) {
		err := executeCommand([]string{"sh", "-c", "kill -TERM $$"}, nil)
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 128+int(syscall.SIGTERM) {
			t.Fatalf("expected ExitError{Code: 143}, got %v", err)
		}
	})

	t.Run("missing executable", func(t *testing.T) {
		err := executeCommand([]string{"coffer-no-such-binary"}, nil)
		if err == nil || !strings.Contains(err.Error(), "failed to start command") {
			t.Fatalf("expected start failure, got %v", err)
		}
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("start failure must not be an ExitError, got %v", err)
		}
	})

	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT} {
		t.Run("forwards "+sig.String(), func(t *testing.T) {
			ready := filepath.Join(t.TempDir(), "ready")
			// Bounded so a missed signal cannot leave the child running past the test.
			script := `trap 'exit 7' HUP INT QUIT TERM; : > "$READY"; i=0; while [ $i -lt 200 ]; do sleep 0.05; i=$((i+1)); done; exit 99`

			done := make(chan error, 1)
			go func() {
				done <- executeCommand([]string{"sh", "-c", script}, map[string]string{"READY": ready})
			}()

			waitForFile(t, ready)
			if err := syscall.Kill(os.Getpid(), sig); err != nil {
				t.Fatalf("failed to signal self: %v", err)
			}

			select {
			case err := <-done:
				var exitErr *ExitError
				if !errors.As(err, &exitErr) || exitErr.Code != 7 {
					t.Fatalf("expected child to trap %s and exit 7, got %v", sig, err)
				}
			case <-time.After(5 * time.Second):
				<-done
				t.Fatalf("child did not exit after %s", sig)
			}
		})
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
