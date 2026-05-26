package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Remote interface {
	Run(ctx context.Context, command string) (CommandResult, error)
	UploadBytes(ctx context.Context, remotePath string, data []byte, mode os.FileMode) error
	UploadFile(ctx context.Context, localPath, remotePath string, mode os.FileMode) error
	MkdirAll(ctx context.Context, remotePath string, mode os.FileMode) error
	Close() error
}

type Dialer interface {
	Dial(ctx context.Context, host Host) (Remote, error)
}

type SSHDialer struct{}

func (d SSHDialer) Dial(ctx context.Context, host Host) (Remote, error) {
	port := host.Port
	if port == 0 {
		port = 22
	}
	auth, err := sshAuth(host)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            host.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host.IP, port))
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, fmt.Sprintf("%s:%d", host.IP, port), cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	client := ssh.NewClient(c, chans, reqs)
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &SSHRemote{client: client, sftp: sftpClient}, nil
}

func sshAuth(host Host) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if host.Password != "" {
		methods = append(methods, ssh.Password(host.Password))
	}
	if host.KeyFile != "" {
		key, err := os.ReadFile(host.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read key_file %s: %w", host.KeyFile, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse key_file %s: %w", host.KeyFile, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("password or key_file is required")
	}
	return methods, nil
}

type SSHRemote struct {
	client *ssh.Client
	sftp   *sftp.Client
}

func (r *SSHRemote) Run(ctx context.Context, command string) (CommandResult, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return CommandResult{ExitCode: 255}, err
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()
	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: 124}, ctx.Err()
	case err := <-done:
		code := 0
		if err != nil {
			code = 1
			if exitErr, ok := err.(*ssh.ExitError); ok {
				code = exitErr.ExitStatus()
			}
		}
		return CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}, err
	}
}

func (r *SSHRemote) MkdirAll(ctx context.Context, remotePath string, mode os.FileMode) error {
	_ = ctx
	if err := r.sftp.MkdirAll(remotePath); err != nil {
		return err
	}
	return r.sftp.Chmod(remotePath, mode)
}

func (r *SSHRemote) UploadBytes(ctx context.Context, remotePath string, data []byte, mode os.FileMode) error {
	_ = ctx
	if err := r.sftp.MkdirAll(path.Dir(remotePath)); err != nil {
		return err
	}
	f, err := r.sftp.OpenFile(remotePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return r.sftp.Chmod(remotePath, mode)
}

func (r *SSHRemote) UploadFile(ctx context.Context, localPath, remotePath string, mode os.FileMode) error {
	data, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer data.Close()
	if err := r.sftp.MkdirAll(path.Dir(remotePath)); err != nil {
		return err
	}
	f, err := r.sftp.OpenFile(remotePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return r.sftp.Chmod(remotePath, mode)
}

func (r *SSHRemote) Close() error {
	var errs []string
	if r.sftp != nil {
		if err := r.sftp.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if r.client != nil {
		if err := r.client.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
