package transfer

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type testSSHServer struct {
	mu                sync.Mutex
	commands          []string
	uploads           int
	bodies            []string
	connections       int
	failUpload        bool
	commandStatus     uint32
	disconnectCommand bool
	listener          net.Listener
	root              string
}

func startSSHServer(t *testing.T, failUpload bool, commandStatus uint32, disconnectCommand bool) *testSSHServer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() != "user" || string(password) != "password" {
			return nil, fmt.Errorf("authentication failed")
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &testSSHServer{listener: listener, failUpload: failUpload, commandStatus: commandStatus, disconnectCommand: disconnectCommand, root: t.TempDir()}
	var wg sync.WaitGroup
	var sockets sync.Map
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			socket, err := listener.Accept()
			if err != nil {
				return
			}
			sockets.Store(socket, true)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer socket.Close()
				defer sockets.Delete(socket)
				conn, channels, requests, err := ssh.NewServerConn(socket, config)
				if err != nil {
					return
				}
				defer conn.Close()
				s.mu.Lock()
				s.connections++
				s.mu.Unlock()
				go ssh.DiscardRequests(requests)
				var channelWG sync.WaitGroup
				defer channelWG.Wait()
				for incoming := range channels {
					if incoming.ChannelType() != "session" {
						incoming.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					channel, requests, err := incoming.Accept()
					if err != nil {
						continue
					}
					channelWG.Add(1)
					go func() { defer channelWG.Done(); defer channel.Close(); s.handleChannel(conn, channel, requests) }()
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		sockets.Range(func(k, _ any) bool { k.(net.Conn).Close(); return true })
		wg.Wait()
	})
	return s
}

func (s *testSSHServer) handleChannel(conn *ssh.ServerConn, channel ssh.Channel, requests <-chan *ssh.Request) {
	for req := range requests {
		if req.Type == "subsystem" {
			var payload struct{ Name string }
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil || payload.Name != "sftp" {
				req.Reply(false, nil)
				return
			}
			req.Reply(true, nil)
			server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(s.root))
			if err != nil {
				return
			}
			defer server.Close()
			server.Serve()
			return
		}
		if req.Type != "exec" {
			req.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
			req.Reply(false, nil)
			return
		}
		req.Reply(true, nil)
		if strings.HasPrefix(payload.Command, "scp -t -- ") {
			s.mu.Lock()
			s.uploads++
			fail := s.failUpload && s.uploads == 1
			s.mu.Unlock()
			channel.Write([]byte{0})
			reader := bufio.NewReader(channel)
			header, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			parts := strings.SplitN(strings.TrimSpace(header), " ", 3)
			if len(parts) != 3 || parts[0] != "C0644" {
				return
			}
			size, err := strconv.Atoi(parts[1])
			if err != nil {
				return
			}
			channel.Write([]byte{0})
			if fail {
				io.CopyN(io.Discard, reader, int64(min(size, 4)))
				conn.Close()
				return
			}
			body := make([]byte, size)
			if _, err := io.ReadFull(reader, body); err != nil {
				return
			}
			if b, err := reader.ReadByte(); err != nil || b != 0 {
				return
			}
			s.mu.Lock()
			s.bodies = append(s.bodies, string(body))
			s.mu.Unlock()
			channel.Write([]byte{0})
			io.Copy(io.Discard, reader)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			return
		}
		s.mu.Lock()
		s.commands = append(s.commands, payload.Command)
		s.mu.Unlock()
		if s.disconnectCommand {
			conn.Close()
			return
		}
		io.WriteString(channel, "remote stdout\n")
		io.WriteString(channel.Stderr(), "remote stderr\n")
		channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{s.commandStatus}))
		return
	}
}

func (s *testSSHServer) args(t *testing.T) Options {
	t.Helper()
	_, portText, _ := net.SplitHostPort(s.listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	return Options{Destination: "user:password@127.0.0.1:/root/trigger", Port: port, Retries: 3, RetryDelay: time.Millisecond, Timeout: time.Second, ConnectTimeout: time.Second}
}

func TestSSHUploadRetryDoesNotRepeatCommands(t *testing.T) {
	s := startSSHServer(t, true, 0, false)
	a := s.args(t)
	payload := strings.Repeat("deployment trigger", 1024)
	a.Sources = []string{writeTestFile(t, filepath.Join(t.TempDir(), "trigger"), payload)}
	a.Precommands = []string{"echo prepare"}
	a.Commands = []string{"/root/cyberme/go-service.sh"}
	if err := handleSSH(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.uploads != 2 || s.connections != 2 || len(s.bodies) != 1 || s.bodies[0] != payload {
		t.Fatalf("uploads=%d connections=%d bodies=%d", s.uploads, s.connections, len(s.bodies))
	}
	if len(s.commands) != 2 || s.commands[0] != "bash -c 'echo prepare'" || s.commands[1] != "bash -c '/root/cyberme/go-service.sh'" {
		t.Fatalf("commands repeated or changed: %v", s.commands)
	}
}

func TestSSHCommandsNeverRetried(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     uint32
		disconnect bool
	}{{"nonzero exit", 7, false}, {"connection lost after dispatch", 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			s := startSSHServer(t, false, tc.status, tc.disconnect)
			a := s.args(t)
			a.Precommands = []string{"deploy once"}
			a.Commands = []string{"must not run"}
			if err := handleSSH(context.Background(), a); err == nil {
				t.Fatal("expected command error")
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if len(s.commands) != 1 || s.connections != 1 {
				t.Fatalf("commands=%v connections=%d", s.commands, s.connections)
			}
		})
	}
}

func TestSSHCommandOnlyAndDisabledRetries(t *testing.T) {
	t.Run("command only", func(t *testing.T) {
		s := startSSHServer(t, false, 0, false)
		a := s.args(t)
		a.Commands = []string{"echo hello"}
		if err := handleSSH(context.Background(), a); err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.commands) != 1 || s.uploads != 0 {
			t.Fatalf("commands=%v uploads=%d", s.commands, s.uploads)
		}
	})
	t.Run("no retry no postcommand", func(t *testing.T) {
		s := startSSHServer(t, true, 0, false)
		a := s.args(t)
		a.Retries = 0
		a.Commands = []string{"must not run"}
		a.Sources = []string{writeTestFile(t, filepath.Join(t.TempDir(), "trigger"), "data")}
		if err := handleSSH(context.Background(), a); err == nil {
			t.Fatal("expected upload failure")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.commands) != 0 || s.uploads != 1 {
			t.Fatalf("commands=%v uploads=%d", s.commands, s.uploads)
		}
	})
}

func TestSSHAuthenticationFailureIsNotRetried(t *testing.T) {
	s := startSSHServer(t, false, 0, false)
	a := s.args(t)
	a.Destination = "user:wrong@127.0.0.1:/tmp/"
	err := handleSSH(context.Background(), a)
	if err == nil || !strings.Contains(err.Error(), "attempt 1/4") {
		t.Fatalf("unexpected auth result: %v", err)
	}
}

func TestSSHDirectoriesAndEmptyFiles(t *testing.T) {
	s := startSSHServer(t, false, 0, false)
	a := s.args(t)
	a.Destination = "user:password@127.0.0.1:deploy/"
	source := filepath.Join(t.TempDir(), "dist")
	writeTestFile(t, filepath.Join(source, "nested", "empty"), "")
	a.Sources = []string{source}
	// Upload twice to ensure pre-existing remote directories do not break retries/reruns.
	for range 2 {
		if err := handleSSH(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
	if info, err := os.Stat(filepath.Join(s.root, "deploy", "dist", "nested")); err != nil || !info.IsDir() {
		t.Fatalf("remote directory missing: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.uploads != 2 || len(s.bodies) != 2 || s.bodies[0] != "" {
		t.Fatalf("empty file transfer failed: uploads=%d bodies=%v", s.uploads, s.bodies)
	}
}

func TestSSHKnownHostsIsOptIn(t *testing.T) {
	s := startSSHServer(t, false, 0, false)
	a := s.args(t)
	// CI needs no known_hosts even when HOME points to a fresh runner directory.
	runnerHome := t.TempDir()
	t.Setenv("HOME", runnerHome)
	t.Setenv("USERPROFILE", runnerHome)
	a.Commands = []string{"deploy"}
	if err := handleSSH(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	a.KnownHosts = filepath.Join(runnerHome, "missing_known_hosts")
	if err := handleSSH(context.Background(), a); err == nil {
		t.Fatal("explicit missing known_hosts should fail")
	}
	a.KnownHosts = writeTestFile(t, filepath.Join(runnerHome, "known_hosts"), "")
	if err := handleSSH(context.Background(), a); err == nil || !strings.Contains(err.Error(), "attempt 1/4") {
		t.Fatalf("unknown host should fail without retries: %v", err)
	}
}
