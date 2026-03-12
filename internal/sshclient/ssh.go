package sshclient

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"turbod/pkg/types"
)

type SSHClient struct {
	client *ssh.Client
	server *types.Server
	config *ssh.ClientConfig
}

func NewSSHClient(server *types.Server) (*SSHClient, error) {
	c := &SSHClient{
		server: server,
	}

	if err := c.buildConfig(); err != nil {
		return nil, err
	}

	return c, nil
}

func (s *SSHClient) buildConfig() error {
	authMethods := []ssh.AuthMethod{}

	if s.server.AuthType == "key" && s.server.KeyPath != "" {
		keyPath := strings.ReplaceAll(s.server.KeyPath, "~", os.Getenv("HOME"))
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("unable to read private key: %v", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return fmt.Errorf("unable to parse private key: %v", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else if s.server.Password != "" {
		authMethods = append(authMethods, ssh.Password(s.server.Password))
	}

	s.config = &ssh.ClientConfig{
		User:            s.server.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	return nil
}

func (s *SSHClient) Connect() error {
	addr := fmt.Sprintf("%s:%d", s.server.Host, s.server.Port)
	client, err := ssh.Dial("tcp", addr, s.config)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %v", addr, err)
	}
	s.client = client
	return nil
}

func (s *SSHClient) Disconnect() {
	if s.client != nil {
		s.client.Close()
	}
}

func (s *SSHClient) Execute(command string) (string, error) {
	if s.client == nil {
		if err := s.Connect(); err != nil {
			return "", err
		}
	}

	session, err := s.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n[STDERR]: " + stderr.String()
	}

	return output, err
}

func (s *SSHClient) UploadFile(localPath, remotePath string) error {
	if s.client == nil {
		if err := s.Connect(); err != nil {
			return err
		}
	}

	remoteDir := filepath.Dir(remotePath)
	_, err := s.Execute(fmt.Sprintf("mkdir -p %s", remoteDir))
	if err != nil {
		return fmt.Errorf("failed to create remote directory: %v", err)
	}

	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %v", err)
	}
	defer file.Close()

	session.Stdin = file
	var stderr bytes.Buffer
	session.Stderr = &stderr

	err = session.Run(fmt.Sprintf("cat > %s", remotePath))
	if err != nil {
		return fmt.Errorf("failed to upload file: %v, stderr: %s", err, stderr.String())
	}

	return nil
}

func (s *SSHClient) UploadContent(content, remotePath string) error {
	remoteDir := filepath.Dir(remotePath)
	_, err := s.Execute(fmt.Sprintf("mkdir -p %s", remoteDir))
	if err != nil {
		return fmt.Errorf("failed to create remote directory: %v", err)
	}

	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %v", err)
	}
	defer stdin.Close()

	go func() {
		fmt.Fprintf(stdin, "%s\n", content)
		stdin.Close()
	}()

	if err = session.Run(fmt.Sprintf("cat > %s", remotePath)); err != nil {
		return fmt.Errorf("failed to upload content: %v", err)
	}

	return nil
}

func (s *SSHClient) RemoteFileExists(remotePath string) bool {
	output, err := s.Execute(fmt.Sprintf("test -e %s && echo 'exists' || echo 'not exists'", remotePath))
	return err == nil && strings.Contains(output, "exists")
}

func (s *SSHClient) SetupPasswordless(keyPath string) error {
	if s.client == nil {
		if err := s.Connect(); err != nil {
			return err
		}
	}

	pubKeyPath := strings.Replace(keyPath, ".pub", "", 1) + ".pub"
	pubKeyPath = strings.ReplaceAll(pubKeyPath, "~", os.Getenv("HOME"))

	pubKeyBytes, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("unable to read public key: %v", err)
	}
	pubKey := strings.TrimSpace(string(pubKeyBytes))

	mkdirCmd := "mkdir -p ~/.ssh && chmod 700 ~/.ssh"
	if _, err := s.Execute(mkdirCmd); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %v", err)
	}

	checkCmd := fmt.Sprintf("grep -q '%s' ~/.ssh/authorized_keys 2>/dev/null || echo '%s' >> ~/.ssh/authorized_keys", pubKey, pubKey)
	if _, err := s.Execute(checkCmd); err != nil {
		return fmt.Errorf("failed to add public key: %v", err)
	}

	chmodCmd := "chmod 600 ~/.ssh/authorized_keys"
	if _, err := s.Execute(chmodCmd); err != nil {
		return fmt.Errorf("failed to set permissions: %v", err)
	}

	return nil
}

func (s *SSHClient) UploadDirectory(localDir, remoteDir string) error {
	if s.client == nil {
		if err := s.Connect(); err != nil {
			return err
		}
	}

	_, err := os.Stat(localDir)
	if err != nil {
		return fmt.Errorf("local directory not found: %v", err)
	}

	_, err = s.Execute(fmt.Sprintf("mkdir -p %s", remoteDir))
	if err != nil {
		return fmt.Errorf("failed to create remote directory: %v", err)
	}

	err = s.uploadDirRecursive(localDir, remoteDir)
	if err != nil {
		return fmt.Errorf("failed to upload directory: %v", err)
	}

	return nil
}

func (s *SSHClient) uploadDirRecursive(localDir, remoteDir string) error {
	entries, err := os.ReadDir(localDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		localPath := filepath.Join(localDir, entry.Name())
		remotePath := filepath.Join(remoteDir, entry.Name())

		if entry.IsDir() {
			_, err := s.Execute(fmt.Sprintf("mkdir -p %s", remotePath))
			if err != nil {
				return err
			}
			if err := s.uploadDirRecursive(localPath, remotePath); err != nil {
				return err
			}
		} else {
			if err := s.UploadFile(localPath, remotePath); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *SSHClient) CheckPort(port int) bool {
	cmd := fmt.Sprintf("nc -z localhost %d 2>/dev/null || echo 'closed'", port)
	output, err := s.Execute(cmd)
	return err == nil && !strings.Contains(output, "closed")
}

type DeploymentClient struct {
	clients map[string]*SSHClient
}

func NewDeploymentClient() *DeploymentClient {
	return &DeploymentClient{
		clients: make(map[string]*SSHClient),
	}
}

func (d *DeploymentClient) GetClient(server *types.Server) (*SSHClient, error) {
	key := fmt.Sprintf("%s:%d", server.Host, server.Port)

	if client, ok := d.clients[key]; ok {
		return client, nil
	}

	client, err := NewSSHClient(server)
	if err != nil {
		return nil, err
	}

	if err := client.Connect(); err != nil {
		return nil, err
	}

	d.clients[key] = client
	return client, nil
}

func (d *DeploymentClient) CloseAll() {
	for _, client := range d.clients {
		client.Disconnect()
	}
	d.clients = make(map[string]*SSHClient)
}
