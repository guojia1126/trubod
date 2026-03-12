package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"turbod/internal/deployer"
	"turbod/internal/scanner"
	"turbod/internal/sshclient"
	"turbod/pkg/types"
	"turbod/pkg/utils"
)

const configFileName = "turbod.json"

type Tab int

const (
	TabServers Tab = iota
	TabConfig
	TabMiddleware
	TabApps
	TabDeploy
)

type Model struct {
	CurrentTab          Tab
	ScannedApps         []types.AppPackage
	Servers             []types.Server
	Middlewares         []types.MiddlewareInstance
	ScanAppsDir         string
	ScanInfraDir        string
	RemoteAppsDir       string
	RemoteMiddlewareDir string
	RemoteStagingDir    string
	Logs                []string
	Progress            int
	TotalTasks          int
	CompletedTasks      int
	IsDistributing      bool
	IsDeploying         bool
	StatusMessage       string

	scanner  *scanner.Scanner
	executor *deployer.DeploymentExecutor
	mu       sync.Mutex
}

func NewModel() *Model {
	return &Model{
		CurrentTab:          TabServers,
		ScannedApps:         []types.AppPackage{},
		Servers:             []types.Server{},
		Middlewares:         []types.MiddlewareInstance{},
		ScanAppsDir:         "./apps",
		ScanInfraDir:        "./infra",
		RemoteAppsDir:       "/opt/apps",
		RemoteMiddlewareDir: "/opt/middleware",
		RemoteStagingDir:    "/opt/staging",
		Logs:                []string{},
		scanner:             scanner.NewScanner(),
	}
}

func (m *Model) AddLog(line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	timestamp := time.Now().Format("15:04:05")
	m.Logs = append(m.Logs, fmt.Sprintf("[%s] %s", timestamp, line))
	if len(m.Logs) > 1000 {
		m.Logs = m.Logs[len(m.Logs)-1000:]
	}
}

func (m *Model) ScanApps() error {
	dir := m.ScanAppsDir
	result, err := m.scanner.ScanAppsDir(dir)
	if err != nil {
		m.AddLog(fmt.Sprintf("扫描应用目录失败: %v", err))
		return err
	}

	m.ScannedApps = result
	m.AddLog(fmt.Sprintf("已扫描 %d 个应用 from %s", len(m.ScannedApps), dir))
	return nil
}

func (m *Model) ScanInfra() error {
	dir := m.ScanInfraDir
	result, err := m.scanner.ScanInfraDir(dir)
	if err != nil {
		m.AddLog(fmt.Sprintf("扫描中间件目录失败: %v", err))
		return err
	}

	m.Middlewares = result
	m.AddLog(fmt.Sprintf("已扫描 %d 个中间件 from %s", len(m.Middlewares), dir))
	return nil
}

func (m *Model) ScanAll() {
	m.ScanApps()
	m.ScanInfra()
}

func (m *Model) AddServer(server types.Server) {
	server.ID = fmt.Sprintf("server-%d", len(m.Servers)+1)
	m.Servers = append(m.Servers, server)
	m.AddLog(fmt.Sprintf("Added server: %s", server.Host))
}

func (m *Model) UpdateServerByHost(host string, updateFunc func(*types.Server)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Servers {
		if m.Servers[i].Host == host {
			updateFunc(&m.Servers[i])
			break
		}
	}
}

func (m *Model) RemoveServer(id string) {
	for i, s := range m.Servers {
		if s.ID == id {
			m.Servers = append(m.Servers[:i], m.Servers[i+1:]...)
			m.AddLog(fmt.Sprintf("Removed server: %s", s.Host))
			break
		}
	}
}

func (m *Model) ToggleServerSelection(id string) {
	for i := range m.Servers {
		if m.Servers[i].ID == id {
			m.Servers[i].Selected = !m.Servers[i].Selected
			break
		}
	}
}

func (m *Model) TestServerConnection(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.Servers {
		if m.Servers[i].ID == id {
			m.Servers[i].Connected = false
			m.Servers[i].LastCheck = time.Now()
			host := m.Servers[i].Host
			m.mu.Unlock()

			client, err := sshclient.NewSSHClient(&m.Servers[i])
			if err != nil {
				m.mu.Lock()
				m.Servers[i].Connected = false
				m.Servers = append(m.Servers[:i], m.Servers[i+1:]...)
				m.Logs = append(m.Logs, fmt.Sprintf("[%s] 连接 %s 失败: %v", time.Now().Format("15:04:05"), host, err))
				return
			}

			if err := client.Connect(); err != nil {
				m.mu.Lock()
				m.Servers[i].Connected = false
				m.Servers = append(m.Servers[:i], m.Servers[i+1:]...)
				m.Logs = append(m.Logs, fmt.Sprintf("[%s] 连接 %s 失败: %v", time.Now().Format("15:04:05"), host, err))
				return
			}

			m.mu.Lock()
			m.Servers[i].Connected = true
			m.Logs = append(m.Logs, fmt.Sprintf("[%s] 连接 %s 成功", time.Now().Format("15:04:05"), host))
			m.mu.Unlock()
			client.Disconnect()
			return
		}
	}
	m.mu.Unlock()
}

func (m *Model) TestAllServerConnections() {
	m.mu.Lock()
	servers := make([]types.Server, len(m.Servers))
	copy(servers, m.Servers)
	m.mu.Unlock()

	for i := range servers {
		servers[i].Connected = false
		servers[i].LastCheck = time.Now()

		client, err := sshclient.NewSSHClient(&servers[i])
		if err != nil {
			m.mu.Lock()
			m.Servers[i].Connected = false
			m.Logs = append(m.Logs, fmt.Sprintf("[%s] 连接 %s 失败: %v", time.Now().Format("15:04:05"), servers[i].Host, err))
			m.mu.Unlock()
			continue
		}

		if err := client.Connect(); err != nil {
			m.mu.Lock()
			m.Servers[i].Connected = false
			m.Logs = append(m.Logs, fmt.Sprintf("[%s] 连接 %s 失败: %v", time.Now().Format("15:04:05"), servers[i].Host, err))
			m.mu.Unlock()
			continue
		}

		m.mu.Lock()
		m.Servers[i].Connected = true
		m.Logs = append(m.Logs, fmt.Sprintf("[%s] 连接 %s 成功", time.Now().Format("15:04:05"), servers[i].Host))
		m.mu.Unlock()
		client.Disconnect()
	}
}

func (m *Model) ToggleAppSelection(name string) {
	for i := range m.ScannedApps {
		if m.ScannedApps[i].Name == name {
			m.ScannedApps[i].Selected = !m.ScannedApps[i].Selected
			break
		}
	}
}

func (m *Model) AddMiddleware(mw types.MiddlewareInstance) {
	m.Middlewares = append(m.Middlewares, mw)
	m.Logs = append(m.Logs, fmt.Sprintf("[%s] 添加中间件: %s", time.Now().Format("15:04:05"), mw.Type))
}

func (m *Model) RemoveMiddleware(idx int) {
	if idx >= 0 && idx < len(m.Middlewares) {
		m.Middlewares = append(m.Middlewares[:idx], m.Middlewares[idx+1:]...)
	}
}

func (m *Model) GetSelectedApps() []types.AppPackage {
	var selected []types.AppPackage
	for _, app := range m.ScannedApps {
		if app.Selected {
			selected = append(selected, app)
		}
	}
	return selected
}

func (m *Model) GetSelectedServers() []types.Server {
	var selected []types.Server
	for _, s := range m.Servers {
		if s.Selected {
			selected = append(selected, s)
		}
	}
	return selected
}

func (m *Model) GetServerByIndex(idx int) *types.Server {
	if idx >= 0 && idx < len(m.Servers) {
		return &m.Servers[idx]
	}
	return nil
}

func (m *Model) GetSelectedMiddlewares() []types.MiddlewareInstance {
	var selected []types.MiddlewareInstance
	for _, mw := range m.Middlewares {
		if mw.Selected && len(mw.TargetServers) > 0 {
			selected = append(selected, mw)
		}
	}
	return selected
}

func (m *Model) StartDeployment() {
	selectedApps := m.GetSelectedApps()
	selectedServers := m.GetSelectedServers()
	selectedMiddlewares := m.GetSelectedMiddlewares()

	if len(selectedApps) == 0 && len(selectedMiddlewares) == 0 {
		m.AddLog("No apps or middleware selected for deployment")
		return
	}

	if len(selectedServers) == 0 {
		m.AddLog("No servers selected for deployment")
		return
	}

	m.IsDeploying = true
	m.TotalTasks = (len(selectedApps) + len(selectedMiddlewares)) * len(selectedServers)
	m.CompletedTasks = 0
	m.Progress = 0

	if m.TotalTasks == 0 {
		m.AddLog("No tasks to deploy (no apps or middleware selected)")
		m.IsDeploying = false
		return
	}

	appCount := len(selectedApps)
	mwCount := len(selectedMiddlewares)
	m.AddLog(fmt.Sprintf("Starting deployment: %d apps + %d middleware to %d servers", appCount, mwCount, len(selectedServers)))

	m.executor = deployer.NewDeploymentExecutor(3)
	go func() {
		m.executor.Deploy(selectedApps, selectedMiddlewares, selectedServers, m.RemoteAppsDir, m.RemoteMiddlewareDir, m.RemoteStagingDir,
			func(task *types.DeploymentTask) {
				m.mu.Lock()
				m.CompletedTasks++
				if m.TotalTasks > 0 {
					m.Progress = (m.CompletedTasks * 100) / m.TotalTasks
					if m.Progress > 100 {
						m.Progress = 100
					}
				}
				m.mu.Unlock()
			},
			func(line string) {
				m.AddLog(line)
			})
		m.mu.Lock()
		m.IsDeploying = false
		m.AddLog("Deployment completed")
		m.mu.Unlock()
	}()
}

func (m *Model) StartDistribution() {
	selectedApps := m.GetSelectedApps()
	selectedServers := m.GetSelectedServers()
	selectedMiddlewares := m.GetSelectedMiddlewares()

	if len(selectedApps) == 0 && len(selectedMiddlewares) == 0 {
		m.AddLog("No apps or middleware selected for distribution")
		return
	}

	if len(selectedServers) == 0 {
		m.AddLog("No servers selected for distribution")
		return
	}

	m.IsDistributing = true
	m.TotalTasks = (len(selectedApps) + len(selectedMiddlewares)) * len(selectedServers)
	m.CompletedTasks = 0
	m.Progress = 0

	if m.TotalTasks == 0 {
		m.AddLog("No tasks to distribute (no apps or middleware selected)")
		m.IsDistributing = false
		return
	}

	appCount := len(selectedApps)
	mwCount := len(selectedMiddlewares)
	m.AddLog(fmt.Sprintf("Starting distribution: %d apps + %d middleware to %d servers", appCount, mwCount, len(selectedServers)))

	m.executor = deployer.NewDeploymentExecutor(3)
	go func() {
		m.executor.Distribute(selectedApps, selectedMiddlewares, selectedServers, m.ScanAppsDir, m.ScanInfraDir, m.RemoteStagingDir,
			func(task *types.DeploymentTask) {
				m.mu.Lock()
				m.CompletedTasks++
				if m.TotalTasks > 0 {
					m.Progress = (m.CompletedTasks * 100) / m.TotalTasks
					if m.Progress > 100 {
						m.Progress = 100
					}
				}
				m.mu.Unlock()
			},
			func(line string) {
				m.AddLog(line)
			})
		m.mu.Lock()
		m.IsDistributing = false
		m.AddLog("Distribution completed")
		m.mu.Unlock()
	}()
}

func (m *Model) StopDeployment() {
	if m.executor != nil {
		m.executor.Close()
	}
	m.IsDeploying = false
	m.IsDistributing = false
	m.AddLog("Operation cancelled")
}

func getConfigPath() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, configFileName)
}

func (m *Model) Save() error {
	path := getConfigPath()
	fmt.Printf("[DEBUG] Saving config to: %s\n", path)

	config := types.Config{
		Servers:             m.Servers,
		Apps:                m.ScannedApps,
		Middlewares:         m.Middlewares,
		ScanAppsDir:         m.ScanAppsDir,
		ScanInfraDir:        m.ScanInfraDir,
		RemoteAppsDir:       m.RemoteAppsDir,
		RemoteMiddlewareDir: m.RemoteMiddlewareDir,
		RemoteStagingDir:    m.RemoteStagingDir,
		MaxParallel:         3,
	}

	for i := range config.Servers {
		if config.Servers[i].Password != "" {
			encrypted, err := utils.Encrypt(config.Servers[i].Password)
			if err != nil {
				fmt.Printf("Warning: Failed to encrypt password: %v\n", err)
			} else {
				config.Servers[i].Password = encrypted
				fmt.Printf("[DEBUG] Encrypted password for server %s\n", config.Servers[i].Host)
			}
		}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %v", err)
	}

	return nil
}

func (m *Model) Load() error {
	path := getConfigPath()
	fmt.Printf("[DEBUG] Loading config from: %s\n", path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read config: %v", err)
	}

	var config types.Config
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %v", err)
	}

	for i := range config.Servers {
		config.Servers[i].Connected = false
		if config.Servers[i].Password != "" {
			decrypted, err := utils.Decrypt(config.Servers[i].Password)
			if err == nil {
				config.Servers[i].Password = decrypted
			}
		}
	}

	m.Servers = config.Servers
	m.ScannedApps = config.Apps
	m.Middlewares = config.Middlewares
	m.ScanAppsDir = config.ScanAppsDir
	m.ScanInfraDir = config.ScanInfraDir
	m.RemoteAppsDir = config.RemoteAppsDir
	m.RemoteMiddlewareDir = config.RemoteMiddlewareDir
	m.RemoteStagingDir = config.RemoteStagingDir

	if m.ScanAppsDir == "" {
		m.ScanAppsDir = "./apps"
	}
	if m.ScanInfraDir == "" {
		m.ScanInfraDir = "./infra"
	}
	if m.RemoteStagingDir == "" {
		m.RemoteStagingDir = "/opt/staging"
	}

	return nil
}
