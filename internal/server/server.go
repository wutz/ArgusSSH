package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"

	"crypto/x509"

	"github.com/wutz/argusssh/internal/config"
	"golang.org/x/crypto/ssh"
)

type Server struct {
	config    *config.Config
	users     map[string]*userAuth
	sshConfig *ssh.ServerConfig
	listener  net.Listener
}

type userAuth struct {
	password string
	commands []string
}

func New(cfg *config.Config) (*Server, error) {
	rendered, err := cfg.RenderUsers()
	if err != nil {
		return nil, fmt.Errorf("rendering user commands: %w", err)
	}

	users := make(map[string]*userAuth)
	for _, ru := range rendered {
		users[ru.Username] = &userAuth{
			password: ru.Password,
			commands: ru.Commands,
		}
		log.Printf("User %s: %d allowed command(s)", ru.Username, len(ru.Commands))
	}

	s := &Server{
		config: cfg,
		users:  users,
	}

	sshConfig := &ssh.ServerConfig{
		PasswordCallback: s.passwordCallback,
	}

	privateKey, err := loadOrGenerateHostKey(cfg.Server.HostKey)
	if err != nil {
		return nil, fmt.Errorf("loading host key: %w", err)
	}

	sshConfig.AddHostKey(privateKey)
	s.sshConfig = sshConfig

	return s, nil
}

func (s *Server) passwordCallback(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	user, ok := s.users[conn.User()]
	if !ok {
		return nil, fmt.Errorf("unknown user")
	}

	if user.password != string(password) {
		return nil, fmt.Errorf("invalid password")
	}

	return &ssh.Permissions{
		Extensions: map[string]string{
			"user": conn.User(),
		},
	}, nil
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.config.Server.Listen)
	if err != nil {
		return fmt.Errorf("starting listener: %w", err)
	}
	s.listener = listener

	log.Printf("SSH server listening on %s", s.config.Server.Listen)

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			default:
				log.Printf("Failed to accept connection: %v", err)
				continue
			}
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *Server) handleConnection(netConn net.Conn) {
	sshConn, chans, reqs, err := ssh.NewServerConn(netConn, s.sshConfig)
	if err != nil {
		log.Printf("Failed to handshake: %v", err)
		return
	}
	defer sshConn.Close()

	username := sshConn.Permissions.Extensions["user"]
	log.Printf("User %s connected from %s", username, sshConn.RemoteAddr())

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Failed to accept channel: %v", err)
			continue
		}

		go s.handleSession(channel, requests, username)
	}
}

func (s *Server) handleSession(channel ssh.Channel, requests <-chan *ssh.Request, username string) {
	defer channel.Close()

	for req := range requests {
		switch req.Type {
		case "exec":
			cmdLine := extractCommand(req.Payload)
			if cmdLine == "" {
				req.Reply(false, nil)
				continue
			}

			log.Printf("User %s exec: %s", username, cmdLine)

			if s.isCommandAllowed(username, cmdLine) {
				req.Reply(true, nil)
				s.executeCommand(channel, cmdLine)
			} else {
				log.Printf("User %s denied: %s", username, cmdLine)
				req.Reply(false, nil)
				fmt.Fprintf(channel.Stderr(), "command not allowed: %s\n", cmdLine)
				channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
			}
			return

		case "shell":
			req.Reply(false, nil)
			fmt.Fprintf(channel.Stderr(), "interactive shell not supported\n")
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
			return

		default:
			req.Reply(false, nil)
		}
	}
}

func extractCommand(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}
	cmdLen := int(payload[0])<<24 | int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	if len(payload) < 4+cmdLen {
		return ""
	}
	return string(payload[4 : 4+cmdLen])
}

func (s *Server) isCommandAllowed(username, cmdLine string) bool {
	user, ok := s.users[username]
	if !ok {
		return false
	}

	for _, allowed := range user.commands {
		if matchCommand(allowed, cmdLine) {
			return true
		}
	}

	return false
}

func matchCommand(pattern, cmdLine string) bool {
	patternParts := strings.Fields(pattern)
	cmdParts := strings.Fields(cmdLine)

	if len(patternParts) == 0 {
		return false
	}

	if len(cmdParts) < len(patternParts) {
		return false
	}

	for i, pp := range patternParts {
		if pp != cmdParts[i] {
			return false
		}
	}

	return true
}

func (s *Server) executeCommand(channel ssh.Channel, cmdLine string) {
	cmd := exec.Command("sh", "-c", cmdLine)
	cmd.Stdout = channel
	cmd.Stderr = channel.Stderr()
	cmd.Stdin = channel

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(exitErr.ExitCode())}))
		} else {
			fmt.Fprintf(channel.Stderr(), "execution error: %v\n", err)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
		}
		return
	}

	channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
}

func loadOrGenerateHostKey(path string) (ssh.Signer, error) {
	if path != "" {
		keyData, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading host key %s: %w", path, err)
		}
		return ssh.ParsePrivateKey(keyData)
	}

	return generateHostKey()
}

func generateHostKey() (ssh.Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ed25519 key: %w", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshaling private key: %w", err)
	}

	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	signer, err := ssh.ParsePrivateKey(pemBlock)
	if err != nil {
		return nil, fmt.Errorf("parsing generated key: %w", err)
	}

	_ = io.Discard
	log.Printf("Generated ephemeral host key (provide host_key in config for persistent key)")
	return signer, nil
}
