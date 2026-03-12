# TurboD - 标准化自动部署工具

## 1. Project Overview

- **Project Name**: TurboD
- **Type**: TUI (Terminal User Interface) Application
- **Framework**: Go + Bubble Tea
- **Core Functionality**: 自动化部署 Spring Boot 应用和中间件到多台远程服务器
- **Target Users**: DevOps 工程师、系统管理员

## 2. UI/UX Specification

### 2.1 Layout Structure

```
┌─────────────────────────────────────────────────────────────┐
│  RurboD - 自动化部署工具                          [ESC]退出  │
├─────────────────────────────────────────────────────────────┤
│  [Tab1: 应用扫描] [Tab2: 服务器配置] [Tab3: 中间件] [Tab4: 部署]│
├─────────────────────────────────────────────────────────────┤
│                                                             │
│                     主内容区域                               │
│                                                             │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│  状态栏: 提示信息 / 进度条                                    │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Visual Design

- **Color Palette**:
  - Primary: `#1E88E5` (Blue)
  - Secondary: `#424242` (Dark Gray)
  - Accent: `#4CAF50` (Green for success)
  - Warning: `#FF9800` (Orange)
  - Error: `#F44336` (Red)
  - Background: `#1E1E1E` (Dark)
  - Text: `#E0E0E0` (Light Gray)

- **Typography**:
  - Title: Bold, 18px
  - Headers: Bold, 14px
  - Body: Regular, 12px
  - Monospace (logs): 11px

### 2.3 Components

1. **Tab Navigation**: 4 tabs for different功能模块
2. **Data Table**: 显示扫描结果/服务器列表，带复选框
3. **Input Form**: 用于输入本地目录、服务器IP等
4. **Progress Bar**: 显示部署进度
5. **Log View**: 实时显示部署日志
6. **Status Bar**: 底部状态提示

## 3. Functionality Specification

### 3.1 应用扫描 (App Scanner)

- **输入**: 用户输入本地根目录路径
- **扫描规则**:
  - 递归查找所有 `*.jar` 文件
  - 同级目录查找 `application.yml`, `application.properties`, `config/` 目录
- **输出**: 表格显示 `[✓] [Jar名] [配置文件] [建议路径]`
- **交互**: 勾选要部署的应用，可编辑远程目录

### 3.2 服务器配置

- **功能**: 管理目标服务器列表
- **数据**: IP地址、SSH端口、用户名、密钥/密码
- **验证**: 连接测试功能

### 3.3 中间件部署

支持以下中间件类型：
- Zookeeper
- Kafka
- Aerospike
- Elasticsearch
- Nacos

**功能**:
- 自动下载/使用指定版本
- 集群配置生成
- 健康检查

### 3.4 部署执行

- **多对多映射**: 多个应用到多台服务器
- **并发执行**: Goroutine 并发上传，限制最大并发数
- **实时日志**: 显示每台服务器的输出
- **进度追踪**: 总体进度条

## 4. Technical Architecture

### 4.1 Data Models

```go
type AppPackage struct {
    Name          string   // 应用名，如 "user-service"
    JarFileName   string   // "user-service-1.0.0.jar"
    LocalJarPath  string   // 本地路径
    ConfigFiles   []string // 配置文件
    RemoteDir     string   // 远程目录
    Selected      bool     // 是否选中
}

type Server struct {
    ID        string
    Host      string
    Port      int
    User      string
    AuthType  string // "password" | "key"
    Password  string
    KeyPath   string
    Selected  bool
}

type DeploymentTask struct {
    App       *AppPackage
    Server    *Server
    Status    string // "pending" | "running" | "success" | "failed"
    Message   string
}
```

### 4.2 Key Modules

1. **Scanner**: 本地JAR包扫描
2. **SSHClient**: SSH/SCP连接
3. **Deployer**: 部署逻辑
4. **Middleware**: 中间件安装
5. **TUI**: Bubble Tea主程序

### 4.3 Embedded Resources

```
templates/
  start.sh       # Spring Boot 启动脚本模板
```

## 5. Acceptance Criteria

- [ ] 可以扫描本地目录，显示所有JAR及其配置文件
- [ ] 可以添加/删除/编辑目标服务器
- [ ] 可以选择要部署的应用和目标服务器
- [ ] 支持并发部署，实时显示进度
- [ ] 支持中间件部署和健康检查
- [ ] 嵌入的启动脚本可正确上传并执行
- [ ] TUI界面流畅，ESC可退出
