package views

import (
	"fmt"
	"strings"

	"turbod/pkg/types"

	"github.com/charmbracelet/lipgloss"
)

var (
	TitleStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#1E88E5")).Bold(true)
	SelectedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#4CAF50"))
	ErrorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#F44336"))
	WarningStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF9800"))
	DimStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#757575"))
	BorderStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1)
	TabActiveStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#1E88E5")).Bold(true).Padding(0, 2)
	TabInactiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#757575")).Padding(0, 2)
)

func RenderTabs(currentTab int, tabs []string) string {
	var tabStrs []string
	for i, tab := range tabs {
		if i == currentTab {
			tabStrs = append(tabStrs, TabActiveStyle.Render("► "+tab))
		} else {
			tabStrs = append(tabStrs, TabInactiveStyle.Render(tab))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabStrs...)
}

func RenderAppsList(apps []types.AppPackage, selectedIdx int, width, height int) string {
	if width < 10 || height < 5 {
		return ""
	}

	var lines []string

	if len(apps) == 0 {
		lines = append(lines, TitleStyle.Render("应用配置"))
		lines = append(lines, DimStyle.Render("  未扫描到应用。按 A 扫描目录。"))
		content := strings.Join(lines, "\n")
		return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
	}

	lines = append(lines, TitleStyle.Render(fmt.Sprintf("应用配置 (已扫描 %d 个)", len(apps))))
	lines = append(lines, DimStyle.Render("  空格 选择 | A 扫描 | D 删除"))
	lines = append(lines, "")

	header := fmt.Sprintf("%-5s %-30s %-20s %-15s", "Sel", "App Name", "Config Files", "Remote Dir")
	lines = append(lines, TitleStyle.Render(header))
	lines = append(lines, strings.Repeat("─", width-2))

	for i, app := range apps {
		isCurrent := i == selectedIdx

		sel := "   "
		if app.Selected {
			sel = " ✓"
		}
		if isCurrent {
			sel = sel + "▶"
		} else {
			sel = sel + " "
		}

		configCount := fmt.Sprintf("%d files", len(app.ConfigFiles))
		remoteDir := app.RemoteDir
		if len(remoteDir) > 15 {
			remoteDir = remoteDir[:12] + "..."
		}

		line := fmt.Sprintf("%s %-30s %-20s %-15s", sel, app.Name, configCount, remoteDir)
		if isCurrent {
			line = SelectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
}

func RenderServersList(servers []types.Server, selectedIdx int, width, height int) string {
	if width < 10 || height < 5 {
		return ""
	}

	if len(servers) == 0 {
		return BorderStyle.Width(width - 2).Height(height - 2).Render(
			DimStyle.Render("未配置服务器。按 N 添加服务器。"),
		)
	}

	var lines []string
	header := fmt.Sprintf("%-4s %-18s %-5s %-10s %-10s %-10s %-12s", "选择", "Host", "Port", "User", "Auth", "免密", "状态")
	lines = append(lines, TitleStyle.Render(header))
	lines = append(lines, strings.Repeat("─", width-2))

	for i, s := range servers {
		isCurrent := i == selectedIdx

		sel := "   "
		if s.Selected {
			sel = " ✓"
		}
		if isCurrent {
			sel = sel + "▶"
		} else {
			sel = sel + " "
		}

		authType := s.AuthType
		if authType == "" {
			authType = "password"
		}

		passwordlessStatus := "  -"
		if s.Passwordless {
			passwordlessStatus = "  ✓"
		}

		status := "未测试    "
		if s.Connected {
			status = "✓ 正常  "
		} else if s.LastCheck.Unix() > 0 {
			status = "✗ 失败  "
		}

		line := fmt.Sprintf("%-4s %-20s %-5d %-10s %-10s %-10s %s", sel, s.Host, s.Port, s.User, authType, passwordlessStatus, status)
		if isCurrent {
			line = SelectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
}

func RenderConfigPanel(scanAppsDir, scanInfraDir, remoteAppsDir, remoteMiddlewareDir, remoteStagingDir string, selectedIdx int, width, height int) string {
	if width < 10 || height < 5 {
		return ""
	}

	configItems := []string{
		"应用扫描目录",
		"中间件扫描目录",
		"应用安装目录",
		"中间件安装目录",
		"物料暂存目录",
	}

	configValues := []string{
		scanAppsDir,
		scanInfraDir,
		remoteAppsDir,
		remoteMiddlewareDir,
		remoteStagingDir,
	}

	var lines []string
	lines = append(lines, TitleStyle.Render("目录配置"))
	lines = append(lines, "")

	for i, label := range configItems {
		isCurrent := selectedIdx == i
		sel := "   "
		if isCurrent {
			sel = SelectedStyle.Render("▶")
		}
		value := configValues[i]
		if len(value) > 40 {
			value = value[:37] + "..."
		}
		line := fmt.Sprintf("%s %-15s %s", sel, label+":", value)
		if isCurrent {
			line = SelectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	lines = append(lines, "")
	lines = append(lines, DimStyle.Render("  ↑↓ 选择 | 回车 修改 | ESC 返回"))

	content := strings.Join(lines, "\n")
	return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
}

func RenderMiddlewareList(mws []types.MiddlewareInstance, servers []types.Server, selectedIdx int, width, height int) string {
	if width < 10 || height < 5 {
		return ""
	}

	var lines []string
	lines = append(lines, TitleStyle.Render("中间件配置"))

	if servers == nil || len(servers) == 0 {
		lines = append(lines, DimStyle.Render("  ⚠ 未配置服务器，请先添加服务器"))
	}
	lines = append(lines, "")

	if len(mws) == 0 {
		lines = append(lines, DimStyle.Render("  (无中间件，请按 A 扫描)"))
	} else {
		lines = append(lines, DimStyle.Render("  空格 选择服务器 | D 删除"))

		for i, mw := range mws {
			isCurrent := selectedIdx == i
			sel := "   "
			if mw.Selected {
				sel = " ✓"
			}
			if isCurrent {
				sel = sel + "▶"
			} else {
				sel = sel + " "
			}

			remoteDir := mw.RemoteDir
			if len(remoteDir) > 25 {
				remoteDir = remoteDir[:22] + "..."
			}

			targetCount := 0
			if servers != nil && len(servers) > 0 {
				for _, s := range servers {
					if isServerSelected(mw.TargetServers, s.Host) {
						targetCount++
					}
				}
			}

			line := fmt.Sprintf("%s %-15s %-25s %d台", sel, mw.Type, remoteDir, targetCount)
			if isCurrent {
				line = SelectedStyle.Render(line)
			}
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")
	return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
}

func isServerSelected(targets []string, host string) bool {
	for _, t := range targets {
		if t == host {
			return true
		}
	}
	return false
}

func RenderServerSelection(mw types.MiddlewareInstance, servers []types.Server, selectedIdx int, width, height int) string {
	if width < 10 || height < 5 {
		return ""
	}

	var lines []string
	lines = append(lines, TitleStyle.Render(fmt.Sprintf("选择部署服务器: %s", mw.Type)))
	lines = append(lines, DimStyle.Render(fmt.Sprintf("远程目录: %s", mw.RemoteDir)))
	lines = append(lines, DimStyle.Render("  空格 勾选/取消 | ↑↓ 选择 | ESC 返回"))
	lines = append(lines, "")

	for i, s := range servers {
		isCurrent := i == selectedIdx

		sel := "   "
		if isServerSelected(mw.TargetServers, s.Host) {
			sel = " ✓"
		}
		if isCurrent {
			sel = sel + "▶"
		} else {
			sel = sel + " "
		}

		line := fmt.Sprintf("%s %s:%d (%s)", sel, s.Host, s.Port, s.User)
		if isCurrent {
			line = SelectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
}

func RenderMiddlewareTypes(mwTypes []string, selectedIdx int, width, height int) string {
	if width < 10 || height < 5 {
		return ""
	}

	var lines []string
	lines = append(lines, TitleStyle.Render("选择中间件类型"))
	lines = append(lines, "")
	lines = append(lines, DimStyle.Render("  ↑/↓ 选择类型 | Enter 或 N 添加"))

	for i, mt := range mwTypes {
		sel := "  "
		if i == selectedIdx {
			sel = SelectedStyle.Render("▶")
		}
		line := fmt.Sprintf("%s %s", sel, mt)
		if i == selectedIdx {
			line = SelectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
}

func RenderDeployPanel(apps []types.AppPackage, servers []types.Server, appsDir, middlewareDir, stagingDir string, selectedApps []types.AppPackage, selectedMiddlewares []types.MiddlewareInstance, width, height int) string {
	if width < 10 || height < 5 {
		return ""
	}

	selectedServers := 0
	for _, s := range servers {
		if s.Selected {
			selectedServers++
		}
	}

	var lines []string
	lines = append(lines, TitleStyle.Render("部署配置"))
	lines = append(lines, "")
	lines = append(lines, TitleStyle.Render("目录配置:"))
	lines = append(lines, fmt.Sprintf("  物料暂存目录:    %s", SelectedStyle.Render(stagingDir)))
	lines = append(lines, fmt.Sprintf("  应用安装目录:    %s", SelectedStyle.Render(appsDir)))
	lines = append(lines, fmt.Sprintf("  中间件安装目录: %s", SelectedStyle.Render(middlewareDir)))
	lines = append(lines, "")

	if len(selectedApps) > 0 {
		lines = append(lines, TitleStyle.Render(fmt.Sprintf("已选应用 (%d个):", len(selectedApps))))
		for _, app := range selectedApps {
			lines = append(lines, fmt.Sprintf("  • %s", app.Name))
		}
		lines = append(lines, "")
	}

	if len(selectedMiddlewares) > 0 {
		lines = append(lines, TitleStyle.Render(fmt.Sprintf("已选中间件 (%d个):", len(selectedMiddlewares))))
		for _, mw := range selectedMiddlewares {
			lines = append(lines, fmt.Sprintf("  • %s", mw.Type))
		}
		lines = append(lines, "")
	}

	lines = append(lines, fmt.Sprintf("  已选服务器: %s", SelectedStyle.Render(fmt.Sprintf("%d 台", selectedServers))))
	lines = append(lines, "")
	lines = append(lines, TitleStyle.Render("操作说明"))
	lines = append(lines, "  • [F] 分发 - 上传应用到服务器")
	lines = append(lines, "  • [D] 部署 - 执行安装脚本")
	lines = append(lines, "  • 空格 选择应用/服务器")
	lines = append(lines, "")
	if selectedServers > 0 && (len(selectedApps) > 0 || len(selectedMiddlewares) > 0) {
		lines = append(lines, SelectedStyle.Render("  ✓ 可以进行分发或部署"))
	} else if selectedServers == 0 {
		lines = append(lines, WarningStyle.Render("  ⚠ 请先选择目标服务器"))
	} else {
		lines = append(lines, WarningStyle.Render("  ⚠ 请先选择应用或中间件"))
	}

	content := strings.Join(lines, "\n")
	return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
}

func RenderLogs(logs []string, width, height int) string {
	if width < 10 || height < 5 {
		return ""
	}

	if len(logs) == 0 {
		return BorderStyle.Width(width - 2).Height(height - 2).Render(
			DimStyle.Render("暂无日志..."),
		)
	}

	start := 0
	if len(logs) > height-4 {
		start = len(logs) - (height - 4)
	}

	displayLogs := logs[start:]
	content := strings.Join(displayLogs, "\n")
	return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
}

func RenderProgressBar(percent int, width int) string {
	barWidth := width - 20
	filled := (barWidth * percent) / 100

	var bar strings.Builder
	bar.WriteString("[")
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar.WriteString("█")
		} else {
			bar.WriteString("░")
		}
	}
	bar.WriteString("]")
	bar.WriteString(fmt.Sprintf(" %d%%", percent))

	if percent == 100 {
		return SelectedStyle.Render(bar.String())
	}
	return TitleStyle.Render(bar.String())
}

func RenderStatusBar(msg string, progress int, width int) string {
	if progress > 0 {
		bar := RenderProgressBar(progress, width/2)
		return lipgloss.JoinHorizontal(lipgloss.Bottom, bar, DimStyle.Render(" "+msg))
	}
	return DimStyle.Render(msg)
}
