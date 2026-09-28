package transfer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshConnection struct {
	client *ssh.Client
	socket net.Conn
	stop   func() bool
}

func (c *sshConnection) close() {
	if c == nil {
		return
	}
	c.stop()
	c.client.Close()
}

func dialSSH(ctx context.Context, c sshConfig, a Options, verify ssh.HostKeyCallback) (*sshConnection, error) {
	dialCtx, cancel := context.WithTimeout(ctx, a.ConnectTimeout)
	defer cancel()
	address := net.JoinHostPort(c.Host, strconv.Itoa(a.Port))
	socket, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(dialCtx, func() { socket.Close() })
	deadline, _ := dialCtx.Deadline()
	if err := socket.SetDeadline(deadline); err != nil {
		stop()
		socket.Close()
		return nil, err
	}
	conn, channels, requests, err := ssh.NewClientConn(socket, address, &ssh.ClientConfig{
		User: c.Username, Auth: []ssh.AuthMethod{ssh.Password(c.Password)}, HostKeyCallback: verify,
	})
	stop()
	if err != nil {
		socket.Close()
		return nil, err
	}
	if err := dialCtx.Err(); err != nil {
		conn.Close()
		return nil, err
	}
	if err := socket.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	client := ssh.NewClient(conn, channels, requests)
	return &sshConnection{client, socket, context.AfterFunc(ctx, func() { client.Close() })}, nil
}

func handleSSH(ctx context.Context, a Options) error {
	c, err := parseSSH(a.Destination)
	if err != nil {
		return err
	}
	plan, err := planTransfers(a.Sources, c.Path, false)
	if err != nil {
		return err
	}
	// Preserve password-only legacy CI configuration. Users may opt into host verification.
	verify := ssh.InsecureIgnoreHostKey()
	if a.KnownHosts != "" {
		verify, err = knownhosts.New(a.KnownHosts)
		if err != nil {
			return fmt.Errorf("load known_hosts: %w", err)
		}
	}
	var connection *sshConnection
	defer func() { connection.close() }()
	connect := func(ctx context.Context) error {
		if connection != nil {
			return nil
		}
		var err error
		connection, err = dialSSH(ctx, c, a, verify)
		return err
	}
	if err := withRetry(ctx, a, "SSH connect", connect); err != nil {
		return err
	}
	if err := executeCommands(connection, a.Precommands, a.CommandTimeout); err != nil {
		return err
	}
	// Reconnect only around file/directory operations; never replay either command list.
	transfer := func(label string, operation func(*sshConnection) error) error {
		return withRetry(ctx, a, label, func(ctx context.Context) error {
			if err := connect(ctx); err != nil {
				return err
			}
			if err := connection.socket.SetDeadline(time.Now().Add(a.Timeout)); err != nil {
				connection.close()
				connection = nil
				return err
			}
			err := operation(connection)
			if err == nil {
				err = connection.socket.SetDeadline(time.Time{})
			}
			if err != nil {
				connection.close()
				connection = nil
			}
			return err
		})
	}
	for _, directory := range plan.Dirs {
		if err := transfer("SSH mkdir "+directory, func(c *sshConnection) error {
			client, err := sftp.NewClient(c.client)
			if err != nil {
				return err
			}
			defer client.Close()
			return client.MkdirAll(directory)
		}); err != nil {
			return err
		}
	}
	for _, file := range plan.Files {
		start := time.Now()
		fmt.Printf("ssh transfer: %s -> %s (%d bytes)\n", file.Local, file.Remote, file.Size)
		if err := transfer("SSH upload "+file.Remote, func(c *sshConnection) error { return uploadSCP(c.client, file) }); err != nil {
			return err
		}
		fmt.Printf("Transferred: %s (%s)\n", file.Remote, time.Since(start).Round(time.Millisecond))
	}
	return executeCommands(connection, a.Commands, a.CommandTimeout)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func executeCommands(c *sshConnection, commands []string, timeout time.Duration) error {
	for _, encoded := range commands {
		command := decodeValue(encoded)
		if command == "" {
			continue
		}
		deadline := time.Time{}
		if timeout > 0 {
			deadline = time.Now().Add(timeout)
		}
		if err := c.socket.SetDeadline(deadline); err != nil {
			return err
		}
		session, err := c.client.NewSession()
		if err != nil {
			return fmt.Errorf("open SSH command session (not retried): %w", err)
		}
		session.Stdout, session.Stderr = os.Stdout, os.Stderr
		fmt.Println("Executing SSH command (no automatic retry)")
		err = session.Run("bash -c " + shellQuote(command))
		session.Close()
		if err != nil {
			return fmt.Errorf("SSH command failed (not retried; it may have executed remotely): %w", err)
		}
		fmt.Println("Exit status: 0")
	}
	return c.socket.SetDeadline(time.Time{})
}

// Keep SCP for file transfers: existing destinations need not enable SFTP for
// single-file uploads. SFTP is used only to create directories, as in Rust.
func uploadSCP(client *ssh.Client, file transferFile) error {
	local, err := os.Open(file.Local)
	if err != nil {
		return err
	}
	defer local.Close()
	info, err := local.Stat()
	if err != nil {
		return err
	}
	name := filepath.Base(file.Local)
	if strings.ContainsAny(name, "\r\n\x00") {
		return fmt.Errorf("SCP filename contains a protocol delimiter")
	}
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	input, err := session.StdinPipe()
	if err != nil {
		return err
	}
	output, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	session.Stderr = os.Stderr
	if err := session.Start("scp -t -- " + shellQuote(file.Remote)); err != nil {
		return err
	}
	reader := bufio.NewReader(output)
	if err := scpAck(reader); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(input, "C0644 %d %s\n", info.Size(), name); err != nil {
		return err
	}
	if err := scpAck(reader); err != nil {
		return err
	}
	if _, err := io.CopyN(input, local, info.Size()); err != nil {
		return err
	}
	if _, err := input.Write([]byte{0}); err != nil {
		return err
	}
	if err := scpAck(reader); err != nil {
		return err
	}
	if err := input.Close(); err != nil {
		return err
	}
	return session.Wait()
}

func scpAck(reader *bufio.Reader) error {
	code, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if code == 0 {
		return nil
	}
	message, _ := bufio.NewReader(io.LimitReader(reader, 4096)).ReadString('\n')
	return fmt.Errorf("SCP error %d: %s", code, strings.TrimSpace(message))
}
