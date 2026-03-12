package middleware

import (
	"fmt"
	"path/filepath"
	"strings"

	"turbod/internal/sshclient"
	"turbod/pkg/types"
)

type MiddlewareDeployer struct {
	client *sshclient.SSHClient
}

func NewMiddlewareDeployer(client *sshclient.SSHClient) *MiddlewareDeployer {
	return &MiddlewareDeployer{client: client}
}

type MiddlewareConfig struct {
	Name         string
	Version      string
	Port         int
	HealthCheck  string
	ClusterPorts map[int]int
}

var MiddlewareConfigs = map[types.MiddlewareType]MiddlewareConfig{
	types.MiddlewareZookeeper: {
		Name:        "zookeeper",
		Port:        2181,
		HealthCheck: "nc -z localhost 2181",
		ClusterPorts: map[int]int{
			2181: 2181,
			2888: 2888,
			3888: 3888,
		},
	},
	types.MiddlewareKafka: {
		Name:        "kafka",
		Port:        9092,
		HealthCheck: "nc -z localhost 9092",
		ClusterPorts: map[int]int{
			9092: 9092,
		},
	},
	types.MiddlewareAerospike: {
		Name:        "aerospike",
		Port:        3000,
		HealthCheck: "nc -z localhost 3000",
		ClusterPorts: map[int]int{
			3000: 3000,
			8081: 8081,
		},
	},
	types.MiddlewareElastic: {
		Name:        "elasticsearch",
		Port:        9200,
		HealthCheck: "curl -s http://localhost:9200 > /dev/null",
		ClusterPorts: map[int]int{
			9200: 9200,
			9300: 9300,
		},
	},
	types.MiddlewareNacos: {
		Name:        "nacos",
		Port:        8848,
		HealthCheck: "curl -s http://localhost:8848/nacos > /dev/null",
		ClusterPorts: map[int]int{
			8848: 8848,
			9848: 9848,
		},
	},
}

func (m *MiddlewareDeployer) Deploy(instance *types.MiddlewareInstance, servers []string, logCallback func(string)) error {
	config, ok := MiddlewareConfigs[instance.Type]
	if !ok {
		return fmt.Errorf("unsupported middleware type: %s", instance.Type)
	}

	version := instance.Version
	if version == "" {
		version = config.Version
	}

	remoteBaseDir := instance.RemoteDir
	if remoteBaseDir == "" {
		remoteBaseDir = fmt.Sprintf("/opt/%s-%s", config.Name, version)
	}

	logCallback(fmt.Sprintf("Starting deployment of %s %s to %d servers", config.Name, version, len(servers)))

	for i, serverHost := range servers {
		nodeID := i + 1
		logCallback(fmt.Sprintf("Deploying to server %s (node %d)", serverHost, nodeID))

		remoteDir := filepath.Join(remoteBaseDir, fmt.Sprintf("node%d", nodeID))

		if err := m.deployNode(instance, config, version, remoteDir, serverHost, nodeID, servers); err != nil {
			logCallback(fmt.Sprintf("ERROR: Failed to deploy to %s: %v", serverHost, err))
			continue
		}

		logCallback(fmt.Sprintf("Successfully deployed to %s", serverHost))
	}

	logCallback("Deployment completed")

	return nil
}

func (m *MiddlewareDeployer) deployNode(instance *types.MiddlewareInstance, config MiddlewareConfig, version, remoteDir, serverHost string, nodeID int, allNodes []string) error {
	cmds := []string{
		fmt.Sprintf("mkdir -p %s", remoteDir),
	}

	for _, cmd := range cmds {
		if _, err := m.client.Execute(cmd); err != nil {
			return fmt.Errorf("failed to setup middleware: %v", err)
		}
	}

	if err := m.generateClusterConfig(instance, config, remoteDir, nodeID, allNodes); err != nil {
		return fmt.Errorf("failed to generate cluster config: %v", err)
	}

	return nil
}

func (m *MiddlewareDeployer) generateClusterConfig(instance *types.MiddlewareInstance, config MiddlewareConfig, remoteDir string, nodeID int, nodes []string) error {
	switch instance.Type {
	case types.MiddlewareZookeeper:
		return m.generateZookeeperConfig(remoteDir, nodeID, nodes)
	case types.MiddlewareKafka:
		return m.generateKafkaConfig(remoteDir, nodeID, nodes)
	case types.MiddlewareElastic:
		return m.generateElasticConfig(remoteDir, nodeID, nodes, instance.CustomConfig)
	case types.MiddlewareNacos:
		return m.generateNacosConfig(remoteDir, nodeID, nodes, instance.CustomConfig)
	}
	return nil
}

func (m *MiddlewareDeployer) generateZookeeperConfig(remoteDir string, nodeID int, nodes []string) error {
	var sb strings.Builder
	sb.WriteString("# Zookeeper Configuration\n")
	sb.WriteString("tickTime=2000\n")
	sb.WriteString("initLimit=10\n")
	sb.WriteString("syncLimit=5\n")
	sb.WriteString(fmt.Sprintf("dataDir=%s/data\n", remoteDir))
	sb.WriteString("clientPort=2181\n")

	for i, node := range nodes {
		sb.WriteString(fmt.Sprintf("server.%d=%s:2888:3888\n", i+1, node))
	}

	zooConf := filepath.Join(remoteDir, "conf", "zoo.cfg")
	return m.client.UploadContent(sb.String(), zooConf)
}

func (m *MiddlewareDeployer) generateKafkaConfig(remoteDir string, nodeID int, nodes []string) error {
	brokerID := nodeID - 1

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("broker.id=%d\n", brokerID))
	sb.WriteString("listeners=PLAINTEXT://:9092\n")
	sb.WriteString("advertised.listeners=PLAINTEXT://:9092\n")
	sb.WriteString("num.network.threads=3\n")
	sb.WriteString("num.io.threads=8\n")
	sb.WriteString("socket.send.buffer.bytes=102400\n")
	sb.WriteString("socket.receive.buffer.bytes=102400\n")
	sb.WriteString("socket.request.max.bytes=104857600\n")
	sb.WriteString(fmt.Sprintf("log.dirs=%s/logs\n", remoteDir))
	sb.WriteString("num.partitions=3\n")
	sb.WriteString("num.recovery.threads.per.data.dir=1\n")
	sb.WriteString("offsets.topic.replication.factor=2\n")
	sb.WriteString("transaction.state.log.replication.factor=1\n")
	sb.WriteString("transaction.state.log.min.isr=1\n")
	sb.WriteString("log.retention.hours=168\n")
	sb.WriteString("log.segment.bytes=1073741824\n")
	sb.WriteString("zookeeper.connect=")

	for i, node := range nodes {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%s:2181", node))
	}
	sb.WriteString("\n")
	sb.WriteString("zookeeper.connection.timeout.ms=18000\n")

	serverProps := filepath.Join(remoteDir, "config", "server.properties")
	return m.client.UploadContent(sb.String(), serverProps)
}

func (m *MiddlewareDeployer) generateElasticConfig(remoteDir string, nodeID int, nodes []string, customConfig map[string]string) error {
	var sb strings.Builder
	sb.WriteString("cluster.name=elasticsearch\n")
	sb.WriteString(fmt.Sprintf("node.name=node-%d\n", nodeID))
	sb.WriteString(fmt.Sprintf("path.data=%s/data\n", remoteDir))
	sb.WriteString(fmt.Sprintf("path.logs=%s/logs\n", remoteDir))
	sb.WriteString("network.host=0.0.0.0\n")
	sb.WriteString("http.port=9200\n")
	sb.WriteString("discovery.seed_hosts=")

	for i, node := range nodes {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%s", node))
	}
	sb.WriteString("\n")

	if len(nodes) > 1 {
		sb.WriteString("cluster.initial_master_nodes=")
		for i := range nodes {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf("node-%d", i+1))
		}
		sb.WriteString("\n")
	}

	for k, v := range customConfig {
		sb.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}

	elasticYml := filepath.Join(remoteDir, "config", "elasticsearch.yml")
	return m.client.UploadContent(sb.String(), elasticYml)
}

func (m *MiddlewareDeployer) generateNacosConfig(remoteDir string, nodeID int, nodes []string, customConfig map[string]string) error {
	var sb strings.Builder
	sb.WriteString("server.port=8848\n")
	sb.WriteString("spring.datasource.platform=mysql\n")
	sb.WriteString("nacos.inetutils.prefer-hostname-over-ip=false\n")
	sb.WriteString("nacos.inetutils.ip-address=127.0.0.1\n")

	if len(nodes) > 1 {
		sb.WriteString("nacos.standalone=false\n")
		sb.WriteString("nacos.cluster.address=")
		for i, node := range nodes {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf("%s:8848", node))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("nacos.standalone=true\n")
	}

	for k, v := range customConfig {
		sb.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}

	appProps := filepath.Join(remoteDir, "nacos", "conf", "application.properties")
	return m.client.UploadContent(sb.String(), appProps)
}

func (m *MiddlewareDeployer) HealthCheck(mwType types.MiddlewareType) (bool, error) {
	config, ok := MiddlewareConfigs[mwType]
	if !ok {
		return false, fmt.Errorf("unsupported middleware type: %s", mwType)
	}

	_, err := m.client.Execute(config.HealthCheck)
	return err == nil, nil
}
