package config

import (
	"fmt"
	"os"
	"text/template"
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

type CommandTemplate struct {
	Name     string   `yaml:"name"`
	Commands []string `yaml:"commands"`
}

type User struct {
	Username  string   `yaml:"username"`
	Password  string   `yaml:"password"`
	Templates []string `yaml:"templates"`
	Params    map[string]string `yaml:"params"`
}

type ServerConfig struct {
	Listen  string `yaml:"listen"`
	HostKey string `yaml:"host_key"`
}

type Config struct {
	Server    ServerConfig      `yaml:"server"`
	Templates []CommandTemplate `yaml:"templates"`
	Users     []User            `yaml:"users"`
}

type RenderedUser struct {
	Username string
	Password string
	Commands []string
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":2222"
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	templateNames := make(map[string]bool)
	for _, t := range c.Templates {
		if t.Name == "" {
			return fmt.Errorf("template name cannot be empty")
		}
		if templateNames[t.Name] {
			return fmt.Errorf("duplicate template name: %s", t.Name)
		}
		templateNames[t.Name] = true
		if len(t.Commands) == 0 {
			return fmt.Errorf("template %q has no commands", t.Name)
		}
	}

	usernames := make(map[string]bool)
	for _, u := range c.Users {
		if u.Username == "" || u.Password == "" {
			return fmt.Errorf("username and password are required")
		}
		if usernames[u.Username] {
			return fmt.Errorf("duplicate username: %s", u.Username)
		}
		usernames[u.Username] = true
		for _, tname := range u.Templates {
			if !templateNames[tname] {
				return fmt.Errorf("user %q references unknown template %q", u.Username, tname)
			}
		}
	}

	return nil
}

func (c *Config) RenderUsers() ([]RenderedUser, error) {
	templateMap := make(map[string][]string)
	for _, t := range c.Templates {
		templateMap[t.Name] = t.Commands
	}

	var rendered []RenderedUser
	for _, u := range c.Users {
		ru := RenderedUser{
			Username: u.Username,
			Password: u.Password,
		}

		for _, tname := range u.Templates {
			cmds := templateMap[tname]
			for _, cmd := range cmds {
				renderedCmd, err := renderTemplate(cmd, u.Params)
				if err != nil {
					return nil, fmt.Errorf("rendering command %q for user %q: %w", cmd, u.Username, err)
				}
				ru.Commands = append(ru.Commands, renderedCmd)
			}
		}

		rendered = append(rendered, ru)
	}

	return rendered, nil
}

func renderTemplate(tmplStr string, params map[string]string) (string, error) {
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr, nil
	}

	tmpl, err := template.New("cmd").Option("missingkey=error").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}
