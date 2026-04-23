package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wutz/argusssh/internal/config"
	"golang.org/x/crypto/ssh"
)

func TestPublicKeyAuth(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatalf("Failed to create SSH public key: %v", err)
	}

	authorizedKey := string(ssh.MarshalAuthorizedKey(sshPubKey))

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen:  ":0",
			HostKey: "",
		},
		Templates: []config.CommandTemplate{
			{Name: "basic", Commands: []string{"echo"}},
		},
		Users: []config.User{
			{
				Username:       "testuser",
				Password:       "",
				AuthorizedKeys: []string{authorizedKey},
				Templates:      []string{"basic"},
				Params:         map[string]string{},
			},
		},
	}

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(privKey)
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	perms, err := srv.publicKeyCallback(mockConnMetadata{user: "testuser"}, signer.PublicKey())
	if err != nil {
		t.Errorf("Public key auth failed: %v", err)
	}
	if perms == nil {
		t.Error("Expected permissions, got nil")
	}
}

func TestPublicKeyAuthUnauthorized(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen:  ":0",
			HostKey: "",
		},
		Templates: []config.CommandTemplate{
			{Name: "basic", Commands: []string{"echo"}},
		},
		Users: []config.User{
			{
				Username:       "testuser",
				Password:       "",
				AuthorizedKeys: []string{},
				Templates:      []string{"basic"},
				Params:         map[string]string{},
			},
		},
	}

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(privKey)
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	_, err = srv.publicKeyCallback(mockConnMetadata{user: "testuser"}, signer.PublicKey())
	if err == nil {
		t.Error("Expected public key auth to fail, but it succeeded")
	}
}

func TestMixedAuth(t *testing.T) {
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatalf("Failed to create SSH public key: %v", err)
	}

	authorizedKey := string(ssh.MarshalAuthorizedKey(sshPubKey))

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen:  ":0",
			HostKey: "",
		},
		Templates: []config.CommandTemplate{
			{Name: "basic", Commands: []string{"echo"}},
		},
		Users: []config.User{
			{
				Username:       "testuser",
				Password:       "testpass",
				AuthorizedKeys: []string{authorizedKey},
				Templates:      []string{"basic"},
				Params:         map[string]string{},
			},
		},
	}

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	perms, err := srv.passwordCallback(mockConnMetadata{user: "testuser"}, []byte("testpass"))
	if err != nil {
		t.Errorf("Password auth failed: %v", err)
	}
	if perms == nil {
		t.Error("Expected permissions, got nil")
	}

	perms, err = srv.publicKeyCallback(mockConnMetadata{user: "testuser"}, sshPubKey)
	if err != nil {
		t.Errorf("Public key auth failed: %v", err)
	}
	if perms == nil {
		t.Error("Expected permissions, got nil")
	}
}

type mockConnMetadata struct {
	user string
}

func (m mockConnMetadata) User() string                                { return m.user }
func (m mockConnMetadata) SessionID() []byte                           { return nil }
func (m mockConnMetadata) ClientVersion() []byte                       { return nil }
func (m mockConnMetadata) ServerVersion() []byte                       { return nil }
func (m mockConnMetadata) RemoteAddr() net.Addr                        { return nil }
func (m mockConnMetadata) LocalAddr() net.Addr                         { return nil }

func TestPublicKeyAuthIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")

	privKey, err := generateTestKey(keyPath)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	pubKeyBytes, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		t.Fatalf("Failed to read public key: %v", err)
	}

	configPath := filepath.Join(tmpDir, "config.yaml")
	configYAML := `
server:
  listen: "127.0.0.1:12223"
  host_key: ""

templates:
  - name: basic
    commands:
      - "echo"

users:
  - username: keyuser
    password: ""
    authorized_keys:
      - "` + string(pubKeyBytes) + `"
    templates:
      - basic
    params: {}
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	go srv.Start()
	defer srv.Stop()

	time.Sleep(1 * time.Second)

	signer, err := ssh.ParsePrivateKey(privKey)
	if err != nil {
		t.Fatalf("Failed to parse private key: %v", err)
	}

	clientConfig := &ssh.ClientConfig{
		User: "keyuser",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	client, err := ssh.Dial("tcp", "127.0.0.1:12223", clientConfig)
	if err != nil {
		t.Fatalf("SSH dial failed: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput("echo test")
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	if string(output) != "test\n" {
		t.Errorf("Expected 'test\\n', got %q", string(output))
	}
}

func generateTestKey(path string) ([]byte, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	if err := os.WriteFile(path, privPEM, 0600); err != nil {
		return nil, err
	}

	pub := priv.Public().(ed25519.PublicKey)
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, err
	}

	pubBytes := ssh.MarshalAuthorizedKey(sshPub)
	if err := os.WriteFile(path+".pub", pubBytes, 0644); err != nil {
		return nil, err
	}

	return privPEM, nil
}
