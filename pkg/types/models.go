package types

import "time"

type AppPackage struct {
	Name         string   `json:"name"`
	JarFileName  string   `json:"jar_file_name"`
	LocalJarPath string   `json:"local_jar_path"`
	ConfigFiles  []string `json:"config_files"`
	RemoteDir    string   `json:"remote_dir"`
	Selected     bool     `json:"selected"`
	Version      string   `json:"version"`
}

type Server struct {
	ID           string    `json:"id"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	User         string    `json:"user"`
	AuthType     string    `json:"auth_type"`
	Password     string    `json:"password"`
	KeyPath      string    `json:"key_path"`
	Selected     bool      `json:"selected"`
	Connected    bool      `json:"-"`
	LastCheck    time.Time `json:"-"`
	Passwordless bool      `json:"passwordless"`
}

type MiddlewareType string

const (
	MiddlewareZookeeper MiddlewareType = "zookeeper"
	MiddlewareKafka     MiddlewareType = "kafka"
	MiddlewareAerospike MiddlewareType = "aerospike"
	MiddlewareElastic   MiddlewareType = "elasticsearch"
	MiddlewareNacos     MiddlewareType = "nacos"
)

type MiddlewareInstance struct {
	Type          MiddlewareType    `json:"type"`
	Version       string            `json:"version"`
	RemoteDir     string            `json:"remote_dir"`
	ConfigFile    string            `json:"config_file"`
	ClusterNodes  []string          `json:"cluster_nodes"`
	CustomConfig  map[string]string `json:"custom_config"`
	TargetServers []string          `json:"target_servers"`
	Selected      bool              `json:"selected"`
}

type DeploymentTask struct {
	ID         string `json:"id"`
	App        *AppPackage
	Server     *Server
	Middleware *MiddlewareInstance
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	Progress   int       `json:"progress"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	LogLines   []string  `json:"-"`
}

type ScanResult struct {
	Apps     []AppPackage
	ScanDir  string
	ScanTime time.Time
	Error    error
}

type Config struct {
	Servers             []Server             `json:"servers"`
	Apps                []AppPackage         `json:"apps"`
	Middlewares         []MiddlewareInstance `json:"middlewares"`
	ScanAppsDir         string               `json:"scan_apps_dir"`
	ScanInfraDir        string               `json:"scan_infra_dir"`
	RemoteAppsDir       string               `json:"remote_apps_dir"`
	RemoteMiddlewareDir string               `json:"remote_middleware_dir"`
	RemoteStagingDir    string               `json:"remote_staging_dir"`
	MaxParallel         int                  `json:"max_parallel"`
}

func DefaultConfig() *Config {
	return &Config{
		Servers:             []Server{},
		Apps:                []AppPackage{},
		Middlewares:         []MiddlewareInstance{},
		ScanAppsDir:         "./apps",
		ScanInfraDir:        "./infra",
		RemoteAppsDir:       "/opt/apps",
		RemoteMiddlewareDir: "/opt/middleware",
		RemoteStagingDir:    "/opt/staging",
		MaxParallel:         3,
	}
}
