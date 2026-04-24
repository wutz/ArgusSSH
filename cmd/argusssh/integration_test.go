package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
server:
  listen: "127.0.0.1:12222"
  host_key: ""

templates:
  - name: basic
    commands:
      - "echo"
      - "date"
      - "whoami"

  - name: file-ops
    commands:
      - "ls"
      - "cat"

users:
  - username: testuser
    password: testpass
    templates:
      - basic
      - file-ops
    params: {}
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/argusssh", "-config", configPath)
	cmd.Dir = filepath.Join("..", "..")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer cmd.Process.Kill()

	time.Sleep(2 * time.Second)

	t.Run("successful auth and allowed command", func(t *testing.T) {
		output, err := sshExec("127.0.0.1:12222", "testuser", "testpass", "echo hello")
		if err != nil {
			t.Fatalf("SSH exec failed: %v", err)
		}
		if output != "hello\n" {
			t.Errorf("Expected 'hello\\n', got %q", output)
		}
	})

	t.Run("failed auth", func(t *testing.T) {
		_, err := sshExec("127.0.0.1:12222", "testuser", "wrongpass", "echo hello")
		if err == nil {
			t.Error("Expected auth failure, got success")
		}
	})

	t.Run("disallowed command", func(t *testing.T) {
		_, err := sshExec("127.0.0.1:12222", "testuser", "testpass", "rm -rf /")
		if err == nil {
			t.Error("Expected command rejection, got success")
		}
	})

	t.Run("allowed command with args", func(t *testing.T) {
		output, err := sshExec("127.0.0.1:12222", "testuser", "testpass", "echo hello world")
		if err != nil {
			t.Fatalf("SSH exec failed: %v", err)
		}
		if output != "hello world\n" {
			t.Errorf("Expected 'hello world\\n', got %q", output)
		}
	})
	t.Run("PTY shell session", func(t *testing.T) {
		config := &ssh.ClientConfig{
			User: "testuser",
			Auth: []ssh.AuthMethod{
				ssh.Password("testpass"),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         5 * time.Second,
		}

		client, err := ssh.Dial("tcp", "127.0.0.1:12222", config)
		if err != nil {
			t.Fatalf("SSH dial failed: %v", err)
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			t.Fatalf("Session creation failed: %v", err)
		}
		defer session.Close()

		modes := ssh.TerminalModes{
			ssh.ECHO: 0,
		}
		if err := session.RequestPty("xterm", 24, 80, modes); err != nil {
			t.Fatalf("PTY request failed: %v", err)
		}

		stdin, err := session.StdinPipe()
		if err != nil {
			t.Fatalf("StdinPipe failed: %v", err)
		}

		var output bytes.Buffer
		session.Stdout = &output

		if err := session.Shell(); err != nil {
			t.Fatalf("Shell request failed: %v", err)
		}

		time.Sleep(500 * time.Millisecond)

		fmt.Fprint(stdin, "echo PTY works\n")
		time.Sleep(500 * time.Millisecond)

		fmt.Fprint(stdin, "exit\n")
		stdin.Close()

		done := make(chan error, 1)
		go func() {
			done <- session.Wait()
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("PTY shell session timed out")
		}

		got := output.String()
		if !bytes.Contains([]byte(got), []byte("PTY works")) {
			t.Errorf("Expected output to contain 'PTY works', got %q", got)
		}
		if !bytes.Contains([]byte(got), []byte("argusssh(testuser)>")) {
			t.Errorf("Expected output to contain prompt 'argusssh(testuser)>', got %q", got)
		}
	})
}

func sshExec(addr, user, password, command string) (string, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return "", fmt.Errorf("dial failed: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("session failed: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout

	if err := session.Run(command); err != nil {
		return "", fmt.Errorf("run failed: %w", err)
	}

	return stdout.String(), nil
}
