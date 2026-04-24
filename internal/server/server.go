package server

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"
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
	password       string
	authorizedKeys []ssh.PublicKey
	commands       []string
}

func New(cfg *config.Config) (*Server, error) {
	rendered, err := cfg.RenderUsers()
	if err != nil {
		return nil, fmt.Errorf("rendering user commands: %w", err)
	}

	users := make(map[string]*userAuth)
	for _, ru := range rendered {
		var pubKeys []ssh.PublicKey
		for _, keyStr := range ru.AuthorizedKeys {
			pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(keyStr))
			if err != nil {
				log.Printf("Warning: failed to parse authorized key for user %s: %v", ru.Username, err)
				continue
			}
			pubKeys = append(pubKeys, pubKey)
		}

		users[ru.Username] = &userAuth{
			password:       ru.Password,
			authorizedKeys: pubKeys,
			commands:       ru.Commands,
		}
		authMethods := []string{}
		if ru.Password != "" {
			authMethods = append(authMethods, "password")
		}
		if len(pubKeys) > 0 {
			authMethods = append(authMethods, fmt.Sprintf("pubkey(%d)", len(pubKeys)))
		}
		log.Printf("User %s: %d allowed command(s), auth: %v", ru.Username, len(ru.Commands), authMethods)
	}

	s := &Server{
		config: cfg,
		users:  users,
	}

	sshConfig := &ssh.ServerConfig{
		PasswordCallback:  s.passwordCallback,
		PublicKeyCallback: s.publicKeyCallback,
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

	if user.password == "" {
		return nil, fmt.Errorf("password auth not enabled for this user")
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

func (s *Server) publicKeyCallback(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	user, ok := s.users[conn.User()]
	if !ok {
		return nil, fmt.Errorf("unknown user")
	}

	if len(user.authorizedKeys) == 0 {
		return nil, fmt.Errorf("public key auth not enabled for this user")
	}

	for _, authorizedKey := range user.authorizedKeys {
		if string(key.Marshal()) == string(authorizedKey.Marshal()) {
			return &ssh.Permissions{
				Extensions: map[string]string{
					"user": conn.User(),
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("public key not authorized")
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

	var ptyWidth, ptyHeight uint32
	var ptyTerm string
	hasPTY := false

	for req := range requests {
		switch req.Type {
		case "pty-req":
			ptyTerm, ptyWidth, ptyHeight = parsePtyRequest(req.Payload)
			hasPTY = true
			req.Reply(true, nil)
			log.Printf("User %s requested PTY: %s %dx%d", username, ptyTerm, ptyWidth, ptyHeight)

		case "window-change":
			if len(req.Payload) >= 8 {
				ptyWidth = binary.BigEndian.Uint32(req.Payload[0:4])
				ptyHeight = binary.BigEndian.Uint32(req.Payload[4:8])
			}
			req.Reply(false, nil)

		case "exec":
			cmdLine := extractCommand(req.Payload)
			if cmdLine == "" {
				req.Reply(false, nil)
				continue
			}

			log.Printf("User %s exec: %s", username, cmdLine)

			if s.isCommandAllowed(username, cmdLine) {
				req.Reply(true, nil)
				if hasPTY {
					s.executeCommandPTY(channel, cmdLine, ptyTerm, ptyWidth, ptyHeight)
				} else {
					s.executeCommand(channel, cmdLine)
				}
				channel.CloseWrite()
			} else {
				log.Printf("User %s denied: %s", username, cmdLine)
				req.Reply(false, nil)
				fmt.Fprintf(channel.Stderr(), "command not allowed: %s\n", cmdLine)
				channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
			}
			return

		case "shell":
			if !hasPTY {
				req.Reply(false, nil)
				fmt.Fprintf(channel.Stderr(), "PTY required for interactive shell\n")
				channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
				return
			}
			req.Reply(true, nil)
			s.handleShell(channel, requests, username, ptyTerm, ptyWidth, ptyHeight)
			return

		default:
			req.Reply(false, nil)
		}
	}
}

func parsePtyRequest(payload []byte) (term string, width, height uint32) {
	if len(payload) < 4 {
		return "", 80, 24
	}
	termLen := binary.BigEndian.Uint32(payload[0:4])
	if uint32(len(payload)) < 4+termLen+8 {
		return "", 80, 24
	}
	term = string(payload[4 : 4+termLen])
	offset := 4 + termLen
	width = binary.BigEndian.Uint32(payload[offset : offset+4])
	height = binary.BigEndian.Uint32(payload[offset+4 : offset+8])
	return term, width, height
}

func (s *Server) handleShell(channel ssh.Channel, requests <-chan *ssh.Request, username string, term string, width, height uint32) {
	go func() {
		for req := range requests {
			switch req.Type {
			case "window-change":
				if len(req.Payload) >= 8 {
					width = binary.BigEndian.Uint32(req.Payload[0:4])
					height = binary.BigEndian.Uint32(req.Payload[4:8])
				}
				req.Reply(false, nil)
			default:
				req.Reply(false, nil)
			}
		}
	}()

	prompt := fmt.Sprintf("argusssh(%s)> ", username)
	fmt.Fprint(channel, "Welcome to ArgusSSH. Type 'help' for available commands, 'exit' to quit.\r\n")
	fmt.Fprint(channel, prompt)

	scanner := bufio.NewScanner(channel)
	scanner.Split(scanCRLFLine)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			fmt.Fprint(channel, prompt)
			continue
		}

		switch line {
		case "exit", "quit", "logout":
			fmt.Fprint(channel, "Goodbye.\r\n")
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			return
		case "help":
			s.printHelp(channel, username)
			fmt.Fprint(channel, prompt)
			continue
		}

		log.Printf("User %s shell: %s", username, line)

		if s.isCommandAllowed(username, line) {
			s.executeCommandShell(channel, line, term, width, height)
		} else {
			log.Printf("User %s denied: %s", username, line)
			fmt.Fprintf(channel, "command not allowed: %s\r\n", line)
		}

		fmt.Fprint(channel, prompt)
	}

	channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
}

func (s *Server) printHelp(channel ssh.Channel, username string) {
	user, ok := s.users[username]
	if !ok {
		return
	}
	fmt.Fprint(channel, "Available commands:\r\n")
	for _, cmd := range user.commands {
		fmt.Fprintf(channel, "  %s\r\n", cmd)
	}
	fmt.Fprint(channel, "\r\nBuiltin commands:\r\n")
	fmt.Fprint(channel, "  help    - Show this help message\r\n")
	fmt.Fprint(channel, "  exit    - Exit the shell\r\n")
}

func scanCRLFLine(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' || data[i] == '\r' {
			token = data[:i]
			advance = i + 1
			if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
				advance = i + 2
			}
			return
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (s *Server) executeCommandPTY(channel ssh.Channel, cmdLine string, term string, width, height uint32) {
	cmd := exec.Command("sh", "-c", cmdLine)
	cmd.Env = append(os.Environ(), fmt.Sprintf("TERM=%s", term))

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(width),
		Rows: uint16(height),
	})
	if err != nil {
		fmt.Fprintf(channel, "execution error: %v\r\n", err)
		channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
		return
	}
	defer ptmx.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(channel, ptmx)
	}()
	go func() {
		io.Copy(ptmx, channel)
	}()

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(exitErr.ExitCode())}))
		} else {
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
		}
	} else {
		channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
	}

	wg.Wait()
}

func (s *Server) executeCommandShell(channel ssh.Channel, cmdLine string, term string, width, height uint32) {
	cmd := exec.Command("sh", "-c", cmdLine)
	cmd.Env = append(os.Environ(), fmt.Sprintf("TERM=%s", term))
	cmd.Stdout = channel
	cmd.Stderr = channel

	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			fmt.Fprintf(channel, "execution error: %v\r\n", err)
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

	log.Printf("Generated ephemeral host key (provide host_key in config for persistent key)")
	return signer, nil
}
