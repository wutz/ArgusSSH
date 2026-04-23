package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	configYAML := `
server:
  listen: ":2222"
  host_key: ""

templates:
  - name: basic
    commands:
      - "echo"
      - "date"
  - name: docker
    commands:
      - "docker ps"
      - "docker logs {{.container}}"

users:
  - username: alice
    password: alice123
    templates:
      - basic
    params: {}
  - username: bob
    password: bob456
    templates:
      - basic
      - docker
    params:
      container: "myapp"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Listen != ":2222" {
		t.Errorf("Expected listen :2222, got %s", cfg.Server.Listen)
	}

	if len(cfg.Templates) != 2 {
		t.Errorf("Expected 2 templates, got %d", len(cfg.Templates))
	}

	if len(cfg.Users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(cfg.Users))
	}
}

func TestRenderUsers(t *testing.T) {
	cfg := &Config{
		Templates: []CommandTemplate{
			{
				Name:     "basic",
				Commands: []string{"echo", "date"},
			},
			{
				Name:     "docker",
				Commands: []string{"docker ps", "docker logs {{.container}}"},
			},
		},
		Users: []User{
			{
				Username:  "alice",
				Password:  "alice123",
				Templates: []string{"basic"},
				Params:    map[string]string{},
			},
			{
				Username:  "bob",
				Password:  "bob456",
				Templates: []string{"basic", "docker"},
				Params:    map[string]string{"container": "myapp"},
			},
		},
	}

	rendered, err := cfg.RenderUsers()
	if err != nil {
		t.Fatalf("RenderUsers failed: %v", err)
	}

	if len(rendered) != 2 {
		t.Fatalf("Expected 2 rendered users, got %d", len(rendered))
	}

	alice := rendered[0]
	if alice.Username != "alice" {
		t.Errorf("Expected alice, got %s", alice.Username)
	}
	if len(alice.Commands) != 2 {
		t.Errorf("Expected 2 commands for alice, got %d", len(alice.Commands))
	}

	bob := rendered[1]
	if bob.Username != "bob" {
		t.Errorf("Expected bob, got %s", bob.Username)
	}
	if len(bob.Commands) != 4 {
		t.Errorf("Expected 4 commands for bob, got %d", len(bob.Commands))
	}

	found := false
	for _, cmd := range bob.Commands {
		if cmd == "docker logs myapp" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected rendered command 'docker logs myapp' not found in bob's commands: %v", bob.Commands)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "duplicate template name",
			cfg: &Config{
				Templates: []CommandTemplate{
					{Name: "basic", Commands: []string{"echo"}},
					{Name: "basic", Commands: []string{"date"}},
				},
				Users: []User{},
			},
			wantErr: true,
		},
		{
			name: "unknown template reference",
			cfg: &Config{
				Templates: []CommandTemplate{
					{Name: "basic", Commands: []string{"echo"}},
				},
				Users: []User{
					{Username: "alice", Password: "pass", Templates: []string{"unknown"}},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate username",
			cfg: &Config{
				Templates: []CommandTemplate{
					{Name: "basic", Commands: []string{"echo"}},
				},
				Users: []User{
					{Username: "alice", Password: "pass1", Templates: []string{"basic"}},
					{Username: "alice", Password: "pass2", Templates: []string{"basic"}},
				},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: &Config{
				Templates: []CommandTemplate{
					{Name: "basic", Commands: []string{"echo"}},
				},
				Users: []User{
					{Username: "alice", Password: "pass", Templates: []string{"basic"}},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
