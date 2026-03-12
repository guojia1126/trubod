package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"turbod/internal/sshclient"
	"turbod/internal/tui/models"
	"turbod/internal/tui/views"
	"turbod/pkg/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	AppStyle      = lipgloss.NewStyle().Padding(1)
	TitleBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1E88E5")).Bold(true).Padding(0, 1)
	HelpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#757575"))
)

type program struct {
	model          *models.Model
	width          int
	height         int
	selectedIdx    int
	inputMode      bool
	inputStep      int
	inputValue     string
	inputPrompt    string
	serverIPs      []string
	serverUser     string
	serverPort     string
	serverPass     string
	serverAuth     string
	serverKeyPath  string
	serverAuthIdx  int
	mwTypeIdx      int
	mwAddStep      int
	mwSelectedType string
	mwSelectMode   bool
	mwSelectIdx    int
	cursorPos      int
}

func main() {
	m := models.NewModel()
	if err := m.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config: %v\n", err)
	}
	p := program{model: m}

	if _, err := tea.NewProgram(p, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := m.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save config: %v\n", err)
	}
}

type tickMsg time.Time

func (p program) Init() tea.Cmd {
	return nil
}

func (p program) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return p, nil

	case tickMsg:
		return p, nil

	case tea.KeyMsg:
		if p.inputMode {
			return p.handleInput(msg)
		}
		return p.handleKey(msg)
	}

	return p, nil
}

func (p program) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if p.model.IsDeploying || p.model.IsDistributing {
			p.model.StopDeployment()
		}
		return p, tea.Quit

	case "esc":
		if p.mwSelectMode {
			p.mwSelectMode = false
			p.mwSelectIdx = 0
			return p, nil
		}
		if p.inputMode {
			p.inputMode = false
			p.inputValue = ""
			p.serverIPs = nil
			return p, nil
		}
		if p.model.IsDeploying || p.model.IsDistributing {
			p.model.StopDeployment()
		}
		return p, tea.Quit

	case "tab", "right", "l":
		p.model.CurrentTab = (p.model.CurrentTab + 1) % 5
		p.selectedIdx = 0
		return p, nil

	case "shift+tab", "left", "h":
		p.model.CurrentTab = (p.model.CurrentTab - 1 + 5) % 5
		p.selectedIdx = 0
		return p, nil

	case "up", "k":
		if p.mwSelectMode {
			if p.mwSelectIdx > 0 {
				p.mwSelectIdx--
			}
		} else {
			if p.selectedIdx > 0 {
				p.selectedIdx--
			}
		}

	case "down", "j":
		if p.mwSelectMode {
			max := len(p.model.Servers) - 1
			if p.mwSelectIdx < max {
				p.mwSelectIdx++
			}
		} else {
			max := p.getMaxIndex()
			if p.selectedIdx < max {
				p.selectedIdx++
			}
		}

	case " ":
		if p.model.CurrentTab == models.TabMiddleware && p.selectedIdx < len(p.model.Middlewares) {
			if p.mwSelectMode {
				if p.mwSelectIdx < len(p.model.Servers) {
					mw := &p.model.Middlewares[p.selectedIdx]
					host := p.model.Servers[p.mwSelectIdx].Host
					found := false
					for i, h := range mw.TargetServers {
						if h == host {
							mw.TargetServers = append(mw.TargetServers[:i], mw.TargetServers[i+1:]...)
							found = true
							break
						}
					}
					if !found {
						mw.TargetServers = append(mw.TargetServers, host)
					}
				}
			} else {
				p.mwSelectMode = true
				p.mwSelectIdx = 0
			}
		} else {
			return p.handleSelection()
		}

	case "a":
		if p.model.CurrentTab == models.TabApps {
			p.model.AddLog("Starting scan apps...")
			if err := p.model.ScanApps(); err != nil {
				p.model.AddLog(fmt.Sprintf("Scan error: %v", err))
			}
		}
		if p.model.CurrentTab == models.TabMiddleware {
			p.model.AddLog("Starting scan infra...")
			if err := p.model.ScanInfra(); err != nil {
				p.model.AddLog(fmt.Sprintf("Scan error: %v", err))
			}
		}

	case "t":
		if p.model.CurrentTab == models.TabServers && len(p.model.Servers) > 0 {
			p.model.AddLog("开始测试服务器连接...")
			go func() {
				p.model.TestAllServerConnections()
				p.model.AddLog("服务器连接测试完成")
			}()
			return p, tea.Sequence(
				tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg { return tickMsg(t) }),
				tea.Tick(time.Millisecond*1000, func(t time.Time) tea.Msg { return tickMsg(t) }),
				tea.Tick(time.Millisecond*1500, func(t time.Time) tea.Msg { return tickMsg(t) }),
			)
		}
		return p, nil

	case "p":
		if p.model.CurrentTab == models.TabServers && len(p.model.Servers) > 0 {
			selectedServer := p.model.GetServerByIndex(p.selectedIdx)
			if selectedServer != nil && selectedServer.AuthType == "key" && selectedServer.KeyPath != "" {
				go func() {
					p.model.AddLog(fmt.Sprintf("Setting up passwordless SSH for %s...", selectedServer.Host))
					client, err := sshclient.NewSSHClient(selectedServer)
					if err != nil {
						p.model.AddLog(fmt.Sprintf("Failed to create SSH client: %v", err))
						return
					}
					if err := client.SetupPasswordless(selectedServer.KeyPath); err != nil {
						p.model.AddLog(fmt.Sprintf("Failed to setup passwordless SSH: %v", err))
					} else {
						p.model.AddLog(fmt.Sprintf("Passwordless SSH setup completed for %s", selectedServer.Host))
					}
				}()
			} else {
				p.model.AddLog("Please select a server with key authentication configured")
			}
		}

	case "n":
		if p.model.CurrentTab == models.TabServers {
			p.inputMode = true
			p.inputStep = 0
			p.inputPrompt = "输入服务器IP (逗号分隔):"
			p.inputValue = ""
			p.cursorPos = 0
			p.serverIPs = nil
			p.serverPort = "22"
			p.serverUser = "root"
			p.serverPass = ""
			p.serverAuth = "password"
			p.serverAuthIdx = 0
		}

	case "enter":
		if p.model.CurrentTab == models.TabDeploy && !p.model.IsDeploying {
			p.model.StartDeployment()
		}
		if p.model.CurrentTab == models.TabConfig {
			switch p.selectedIdx {
			case 0:
				p.inputMode = true
				p.inputStep = 20
				p.inputPrompt = fmt.Sprintf("设置应用扫描目录 (当前: %s):", p.model.ScanAppsDir)
				p.inputValue = ""
				p.cursorPos = 0
			case 1:
				p.inputMode = true
				p.inputStep = 30
				p.inputPrompt = fmt.Sprintf("设置中间件扫描目录 (当前: %s):", p.model.ScanInfraDir)
				p.inputValue = ""
				p.cursorPos = 0
			case 2:
				p.inputMode = true
				p.inputStep = 0
				p.inputPrompt = fmt.Sprintf("设置应用安装目录 (当前: %s):", p.model.RemoteAppsDir)
				p.inputValue = ""
				p.cursorPos = 0
			case 3:
				p.inputMode = true
				p.inputStep = 10
				p.inputPrompt = fmt.Sprintf("设置中间件安装目录 (当前: %s):", p.model.RemoteMiddlewareDir)
				p.inputValue = ""
				p.cursorPos = 0
			case 4:
				p.inputMode = true
				p.inputStep = 40
				p.inputPrompt = fmt.Sprintf("设置物料暂存目录 (当前: %s):", p.model.RemoteStagingDir)
				p.inputValue = ""
				p.cursorPos = 0
			}
		}

	case "d":
		if p.model.CurrentTab == models.TabServers && p.selectedIdx < len(p.model.Servers) {
			p.model.RemoveServer(p.model.Servers[p.selectedIdx].ID)
		}
		if p.model.CurrentTab == models.TabMiddleware && p.selectedIdx < len(p.model.Middlewares) {
			p.model.RemoveMiddleware(p.selectedIdx)
		}
		if p.model.CurrentTab == models.TabDeploy && !p.model.IsDeploying && !p.model.IsDistributing {
			p.model.StartDeployment()
		}

	case "f":
		if p.model.CurrentTab == models.TabDeploy && !p.model.IsDeploying && !p.model.IsDistributing {
			p.model.StartDistribution()
		}
	}

	return p, nil
}

func (p program) handleInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if p.model.CurrentTab == models.TabServers {
			if p.inputStep == 0 {
				ips := parseIPs(p.inputValue)
				if len(ips) > 0 {
					p.serverIPs = ips
					p.inputStep = 1
					p.inputValue = ""
					p.cursorPos = 0
					p.inputPrompt = fmt.Sprintf("已添加 %d 台服务器, 输入统一用户名:", len(ips))
					return p, nil
				}
			} else if p.inputStep == 1 {
				p.serverUser = p.inputValue
				p.inputStep = 2
				p.serverAuthIdx = 0
				p.inputPrompt = "认证方式:"
				p.inputValue = ""
				return p, nil
			} else if p.inputStep == 2 {
				if p.serverAuthIdx == 0 {
					p.serverAuth = "password"
				} else {
					p.serverAuth = "key"
				}
				if p.serverAuth == "key" {
					p.inputStep = 3
					p.inputValue = "~/.ssh/id_rsa"
					p.cursorPos = len(p.inputValue)
					p.inputPrompt = "SSH密钥路径:"
					return p, nil
				} else {
					p.inputStep = 3
					p.inputValue = p.serverPass
					p.cursorPos = len(p.inputValue)
					p.inputPrompt = fmt.Sprintf("已添加 %d 台服务器, 输入统一密码:", len(p.serverIPs))
					return p, nil
				}
			} else if p.inputStep == 3 {
				if p.serverAuth == "key" {
					p.serverKeyPath = p.inputValue
					keyPath := strings.ReplaceAll(p.inputValue, "~", os.Getenv("HOME"))
					if _, err := os.Stat(keyPath); os.IsNotExist(err) {
						p.model.AddLog(fmt.Sprintf("密钥不存在，正在生成..."))
						cmd := exec.Command("ssh-keygen", "-t", "rsa", "-b", "4096", "-f", keyPath, "-N", "", "-C", "turbod")
						if err := cmd.Run(); err != nil {
							p.model.AddLog(fmt.Sprintf("密钥生成失败: %v", err))
						} else {
							p.model.AddLog(fmt.Sprintf("密钥生成成功: %s", keyPath))
						}
					}
				} else {
					p.serverPass = p.inputValue
				}
				p.inputStep = 4
				p.inputValue = p.serverPort
				p.cursorPos = len(p.inputValue)
				p.inputPrompt = fmt.Sprintf("已添加 %d 台服务器, 输入SSH端口:", len(p.serverIPs))
				return p, nil
			} else if p.inputStep == 4 {
				p.serverPort = p.inputValue
				if p.serverAuth == "key" {
					p.inputStep = 5
					p.inputValue = "y"
					p.cursorPos = len(p.inputValue)
					p.inputPrompt = "是否配置免密登录? (Y/n):"
					return p, nil
				} else {
					for _, ip := range p.serverIPs {
						port := 22
						fmt.Sscanf(p.serverPort, "%d", &port)
						server := types.Server{
							Host:     ip,
							Port:     port,
							User:     p.serverUser,
							Password: p.serverPass,
							AuthType: p.serverAuth,
							KeyPath:  p.serverKeyPath,
							Selected: true,
						}
						p.model.AddServer(server)
					}
					p.model.AddLog(fmt.Sprintf("批量添加了 %d 台服务器 (认证方式: %s)", len(p.serverIPs), p.serverAuth))
					p.inputMode = false
					p.inputValue = ""
					p.serverIPs = nil
					return p, tea.Sequence(
						tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg { return tickMsg(t) }),
						tea.Tick(time.Millisecond*300, func(t time.Time) tea.Msg { return tickMsg(t) }),
					)
				}
			} else if p.inputStep == 5 {
				if strings.ToLower(strings.TrimSpace(p.inputValue)) == "y" || p.inputValue == "" {
					p.inputStep = 6
					p.inputValue = ""
					p.cursorPos = 0
					p.inputPrompt = fmt.Sprintf("请输入密码用于配置免密登录:")
					return p, nil
				} else {
					for _, ip := range p.serverIPs {
						port := 22
						fmt.Sscanf(p.serverPort, "%d", &port)
						server := types.Server{
							Host:     ip,
							Port:     port,
							User:     p.serverUser,
							Password: p.serverPass,
							AuthType: p.serverAuth,
							KeyPath:  p.serverKeyPath,
							Selected: true,
						}
						p.model.AddServer(server)
					}
					p.model.AddLog(fmt.Sprintf("批量添加了 %d 台服务器 (认证方式: %s, 免密: 否)", len(p.serverIPs), p.serverAuth))
					p.inputMode = false
					p.inputValue = ""
					p.serverIPs = nil
					return p, tea.Sequence(
						tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg { return tickMsg(t) }),
						tea.Tick(time.Millisecond*300, func(t time.Time) tea.Msg { return tickMsg(t) }),
					)
				}
			} else if p.inputStep == 6 {
				tempPassword := p.inputValue
				serverIPs := make([]string, len(p.serverIPs))
				copy(serverIPs, p.serverIPs)
				serverUser := p.serverUser
				serverPort := p.serverPort
				serverKeyPath := p.serverKeyPath
				for _, ip := range serverIPs {
					port := 22
					fmt.Sscanf(serverPort, "%d", &port)
					server := types.Server{
						Host:         ip,
						Port:         port,
						User:         serverUser,
						Password:     tempPassword,
						AuthType:     "password",
						KeyPath:      serverKeyPath,
						Selected:     true,
						Passwordless: false,
					}
					p.model.AddServer(server)
				}
				p.model.AddLog(fmt.Sprintf("已添加 %d 台服务器", len(serverIPs)))
				p.inputMode = false
				p.inputValue = ""
				p.serverIPs = nil
				go func() {
					p.model.AddLog(fmt.Sprintf("将为 %d 台服务器配置免密登录...", len(serverIPs)))
					keyCount := 0
					for _, ip := range serverIPs {
						port := 22
						fmt.Sscanf(serverPort, "%d", &port)
						server := types.Server{
							Host:     ip,
							Port:     port,
							User:     serverUser,
							Password: tempPassword,
							AuthType: "password",
							KeyPath:  serverKeyPath,
						}
						client, err := sshclient.NewSSHClient(&server)
						if err != nil {
							p.model.AddLog(fmt.Sprintf("连接服务器 %s 失败: %v", ip, err))
							continue
						}
						if err := client.SetupPasswordless(serverKeyPath); err != nil {
							p.model.AddLog(fmt.Sprintf("服务器 %s 免密配置失败: %v", ip, err))
						} else {
							p.model.AddLog(fmt.Sprintf("服务器 %s 免密配置成功", ip))
							p.model.UpdateServerByHost(ip, func(s *types.Server) {
								s.Passwordless = true
								s.AuthType = "key"
								s.Password = ""
							})
							keyCount++
						}
					}
					if keyCount > 0 {
						p.model.AddLog(fmt.Sprintf("免密配置成功: %d 台", keyCount))
					}
				}()
				return p, tea.Sequence(
					tea.Tick(time.Millisecond*300, func(t time.Time) tea.Msg { return tickMsg(t) }),
					tea.Tick(time.Millisecond*600, func(t time.Time) tea.Msg { return tickMsg(t) }),
					tea.Tick(time.Millisecond*900, func(t time.Time) tea.Msg { return tickMsg(t) }),
					tea.Tick(time.Millisecond*1200, func(t time.Time) tea.Msg { return tickMsg(t) }),
					tea.Tick(time.Millisecond*1500, func(t time.Time) tea.Msg { return tickMsg(t) }),
					tea.Tick(time.Millisecond*2000, func(t time.Time) tea.Msg { return tickMsg(t) }),
					tea.Tick(time.Millisecond*2500, func(t time.Time) tea.Msg { return tickMsg(t) }),
				)
			}
			if p.inputValue != "" {
				p.model.ScanAppsDir = p.inputValue
				p.model.AddLog(fmt.Sprintf("Set apps scan directory: %s", p.inputValue))
			}
			p.inputMode = false
			p.inputValue = ""
		} else if p.model.CurrentTab == models.TabMiddleware && p.inputMode {
			if p.mwAddStep == 0 {
				remoteDir := p.inputValue
				if remoteDir == "" {
					remoteDir = fmt.Sprintf("%s/%s", p.model.RemoteMiddlewareDir, p.mwSelectedType)
				}
				mw := types.MiddlewareInstance{
					Type:      types.MiddlewareType(p.mwSelectedType),
					Version:   "",
					RemoteDir: remoteDir,
					Selected:  true,
				}
				p.model.AddMiddleware(mw)
				p.model.AddLog(fmt.Sprintf("添加中间件: %s -> %s", p.mwSelectedType, remoteDir))
				p.inputMode = false
				p.inputValue = ""
				p.mwSelectedType = ""
			}
		} else if p.model.CurrentTab == models.TabConfig {
			if p.inputStep == 0 {
				if p.inputValue != "" {
					p.model.RemoteAppsDir = p.inputValue
					p.model.AddLog(fmt.Sprintf("应用安装目录设置为: %s", p.inputValue))
				}
				p.inputMode = false
				p.inputValue = ""
			} else if p.inputStep == 10 {
				if p.inputValue != "" {
					p.model.RemoteMiddlewareDir = p.inputValue
					p.model.AddLog(fmt.Sprintf("中间件安装目录设置为: %s", p.inputValue))
				}
				p.inputMode = false
				p.inputValue = ""
			} else if p.inputStep == 20 {
				if p.inputValue != "" {
					p.model.ScanAppsDir = p.inputValue
					p.model.AddLog(fmt.Sprintf("应用扫描目录设置为: %s", p.inputValue))
				}
				p.inputMode = false
				p.inputValue = ""
			} else if p.inputStep == 30 {
				if p.inputValue != "" {
					p.model.ScanInfraDir = p.inputValue
					p.model.AddLog(fmt.Sprintf("中间件扫描目录设置为: %s", p.inputValue))
				}
				p.inputMode = false
				p.inputValue = ""
			} else if p.inputStep == 40 {
				if p.inputValue != "" {
					p.model.RemoteStagingDir = p.inputValue
					p.model.AddLog(fmt.Sprintf("物料暂存目录设置为: %s", p.inputValue))
				}
				p.inputMode = false
				p.inputValue = ""
			}
		}

	case "esc":
		p.inputMode = false
		p.inputValue = ""
		p.cursorPos = 0
		p.serverIPs = nil
		p.mwSelectMode = false
		p.mwSelectIdx = 0

	case "backspace":
		if len(p.inputValue) > 0 {
			p.inputValue = p.inputValue[:len(p.inputValue)-1]
		}

	case "left":
		if p.cursorPos > 0 {
			p.cursorPos--
		}

	case "right":
		if p.cursorPos < len(p.inputValue) {
			p.cursorPos++
		}

	case "home":
		p.cursorPos = 0

	case "end":
		p.cursorPos = len(p.inputValue)

	case "up", "k":
		if p.model.CurrentTab == models.TabServers && p.inputStep == 2 && len(p.serverIPs) > 0 {
			if p.serverAuthIdx > 0 {
				p.serverAuthIdx--
			}
		}

	case "down", "j":
		if p.model.CurrentTab == models.TabServers && p.inputStep == 2 && len(p.serverIPs) > 0 {
			if p.serverAuthIdx < 1 {
				p.serverAuthIdx++
			}
		}

	case "insert", "pageup", "pagedown":

	default:
		// Insert character at cursor position
		if p.cursorPos < 0 {
			p.cursorPos = 0
		}
		if p.cursorPos > len(p.inputValue) {
			p.cursorPos = len(p.inputValue)
		}
		if p.cursorPos == len(p.inputValue) {
			p.inputValue += msg.String()
		} else {
			p.inputValue = p.inputValue[:p.cursorPos] + msg.String() + p.inputValue[p.cursorPos:]
		}
		p.cursorPos++
	}

	return p, nil
}

func (p program) handleSelection() (tea.Model, tea.Cmd) {
	switch p.model.CurrentTab {
	case models.TabApps:
		if p.selectedIdx < len(p.model.ScannedApps) {
			name := p.model.ScannedApps[p.selectedIdx].Name
			p.model.ToggleAppSelection(name)
		}

	case models.TabServers:
		if p.selectedIdx < len(p.model.Servers) {
			id := p.model.Servers[p.selectedIdx].ID
			p.model.ToggleServerSelection(id)
		}

	case models.TabMiddleware:
		if p.selectedIdx < len(p.model.Middlewares) {
			mw := &p.model.Middlewares[p.selectedIdx]
			if len(mw.TargetServers) == len(p.model.Servers) {
				mw.TargetServers = nil
			} else {
				mw.TargetServers = nil
				for _, s := range p.model.Servers {
					mw.TargetServers = append(mw.TargetServers, s.Host)
				}
			}
		}
	}
	return p, nil
}

func (p program) getMaxIndex() int {
	switch p.model.CurrentTab {
	case models.TabApps:
		if len(p.model.ScannedApps) > 0 {
			return len(p.model.ScannedApps) - 1
		}
	case models.TabServers:
		if len(p.model.Servers) > 0 {
			return len(p.model.Servers) - 1
		}
	case models.TabConfig:
		return 4
	case models.TabMiddleware:
		if len(p.model.Middlewares) > 0 {
			return len(p.model.Middlewares) - 1
		}
	}
	return 0
}

func (p program) View() string {
	tabs := []string{"服务器", "配置", "中间件", "应用", "部署"}
	tabBar := views.RenderTabs(int(p.model.CurrentTab), tabs)

	var mainContent string

	if p.inputMode {
		mainContent = p.renderInputForm()
	} else {
		mainContent = p.renderMainContent()
	}

	progress := 0
	if p.model.TotalTasks > 0 {
		progress = p.model.Progress
	}

	status := p.model.StatusMessage
	if p.model.IsDistributing {
		status = fmt.Sprintf("分发中... %d/%d 任务完成", p.model.CompletedTasks, p.model.TotalTasks)
	} else if p.model.IsDeploying {
		status = fmt.Sprintf("部署中... %d/%d 任务完成", p.model.CompletedTasks, p.model.TotalTasks)
	}

	var help string
	switch p.model.CurrentTab {
	case models.TabServers:
		help = HelpStyle.Render(" [←→]切换 | [空格]选择 | [N]添加 | [D]删除 | [T]测试 | [P]免密配置 | [Esc]退出")
	case models.TabConfig:
		help = HelpStyle.Render(" [←→]切换 | [↑↓]选择 | [回车]修改配置 | [Esc]取消")
	case models.TabMiddleware:
		help = HelpStyle.Render(" [←→]切换 | [↑↓]选择 | [空格]选择服务器 | [A]扫描 | [D]删除 | [Esc]退出")
	case models.TabApps:
		help = HelpStyle.Render(" [←→]切换 | [空格]选择 | [A]扫描 | [D]删除 | [Esc]退出")
	case models.TabDeploy:
		help = HelpStyle.Render(" [←→]切换 | [空格]选择 | [F]分发 | [D]部署 | [Esc]退出")
	}

	logoLines := []string{
		"",
		" ████████╗██╗░░░██╗██████╗░██████╗░░█████╗░██████╗░ ",
		" ╚══██╔══╝██║░░░██║██╔══██╗██╔══██╗██╔══██╗██╔══██╗ ",
		" ░░░██║░░░██║░░░██║██████╔╝██████╦╝██║░░██║██║░░██║ ",
		" ░░░██║░░░██║░░░██║██╔══██╗██╔══██╗██║░░██║██║░░██║ ",
		" ░░░██║░░░╚██████╔╝██║░░██║██████╦╝╚█████╔╝██████╔╝ ",
		" ░░░╚═╝░░░░╚═════╝░╚═╝░░╚═╝╚═════╝░░╚════╝░╚═════╝░ ",
		"",
	}

	logoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#58a6ff")).
		Bold(false).
		Padding(0, 0)

	logo := ""
	for _, line := range logoLines {
		logo += logoStyle.Render(line) + "\n"
	}

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(
			logo +
				tabBar + "\n" +
				mainContent + "\n" +
				views.RenderStatusBar(status, progress, p.width) + "\n" +
				help,
		)
}

func (p program) renderInputForm() string {
	var displayValue string
	if p.cursorPos < 0 {
		p.cursorPos = 0
	}
	if p.cursorPos > len(p.inputValue) {
		p.cursorPos = len(p.inputValue)
	}

	if p.model.CurrentTab == models.TabServers && p.inputStep == 2 && len(p.serverIPs) > 0 {
		authOptions := []string{"password", "key"}
		var optionsDisplay string
		for i, opt := range authOptions {
			if i == p.serverAuthIdx {
				optionsDisplay += fmt.Sprintf("  ○ %s\n", opt)
			} else {
				optionsDisplay += fmt.Sprintf("   %s\n", opt)
			}
		}
		return fmt.Sprintf("%s\n\n%s\n[↑↓]选择 | [Enter] 确认 | [Esc] 取消", p.inputPrompt, optionsDisplay)
	}

	if p.cursorPos == len(p.inputValue) {
		displayValue = p.inputValue + "█"
	} else {
		displayValue = p.inputValue[:p.cursorPos] + "█" + p.inputValue[p.cursorPos:]
	}

	if p.model.CurrentTab == models.TabServers && p.inputStep > 0 && len(p.serverIPs) > 0 {
		return fmt.Sprintf("%s\n\n> %s\n\n[Enter] 继续 | [Esc] 取消", p.inputPrompt, displayValue)
	}
	if p.model.CurrentTab == models.TabServers {
		return fmt.Sprintf("%s\n\n> %s\n\n[Enter] 添加 | [Esc] 取消", p.inputPrompt, displayValue)
	}
	return fmt.Sprintf("%s\n\n> %s\n\n[Enter] 提交 | [Esc] 取消", p.inputPrompt, displayValue)
}

func parseIPs(input string) []string {
	var ips []string
	input = strings.ReplaceAll(input, "[", "")
	input = strings.ReplaceAll(input, "]", "")
	parts := strings.Split(input, ",")
	for _, part := range parts {
		ip := strings.TrimSpace(part)
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips
}

func (p program) renderMainContent() string {
	contentHeight := p.height - 22

	switch p.model.CurrentTab {
	case models.TabApps:
		return views.RenderAppsList(p.model.ScannedApps, p.selectedIdx, p.width, contentHeight)

	case models.TabServers:
		return views.RenderServersList(p.model.Servers, p.selectedIdx, p.width, contentHeight)

	case models.TabConfig:
		return views.RenderConfigPanel(p.model.ScanAppsDir, p.model.ScanInfraDir, p.model.RemoteAppsDir, p.model.RemoteMiddlewareDir, p.model.RemoteStagingDir, p.selectedIdx, p.width, contentHeight)

	case models.TabMiddleware:
		if p.mwSelectMode && p.selectedIdx < len(p.model.Middlewares) {
			return views.RenderServerSelection(p.model.Middlewares[p.selectedIdx], p.model.Servers, p.mwSelectIdx, p.width, contentHeight)
		}
		return views.RenderMiddlewareList(p.model.Middlewares, p.model.Servers, p.selectedIdx, p.width, contentHeight)

	case models.TabDeploy:
		selectedApps := p.model.GetSelectedApps()
		selectedMiddlewares := p.model.GetSelectedMiddlewares()
		deployPanel := views.RenderDeployPanel(p.model.ScannedApps, p.model.Servers, p.model.RemoteAppsDir, p.model.RemoteMiddlewareDir, p.model.RemoteStagingDir, selectedApps, selectedMiddlewares, p.width/2, contentHeight/2)
		logsPanel := views.RenderLogs(p.model.Logs, p.width/2, contentHeight/2)
		return lipgloss.JoinHorizontal(
			lipgloss.Top,
			deployPanel,
			logsPanel,
		)
	}

	return ""
}
