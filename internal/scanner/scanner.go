package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"turbod/pkg/types"
)

var (
	jarRegex       = regexp.MustCompile(`^(.+?)-(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.]+)?)\.jar$`)
	simpleJarRegex = regexp.MustCompile(`^([^-\.]+)\.jar$`)
)

type Scanner struct {
	scanDir string
}

func NewScanner() *Scanner {
	return &Scanner{}
}

func (s *Scanner) ScanAppsDir(dir string) ([]types.AppPackage, error) {
	s.scanDir = dir
	var apps []types.AppPackage

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return apps, fmt.Errorf("directory does not exist: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return apps, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		appDir := filepath.Join(dir, entry.Name())
		apps = s.scanAppDir(appDir, entry.Name())
	}

	return apps, nil
}

func (s *Scanner) scanAppDir(appDir, appName string) []types.AppPackage {
	var apps []types.AppPackage

	entries, _ := os.ReadDir(appDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if strings.HasSuffix(entry.Name(), ".jar") {
			jarPath := filepath.Join(appDir, entry.Name())
			app := s.processJarFileFromDir(jarPath, appDir, appName)
			if app != nil {
				apps = append(apps, *app)
			}
		}
	}

	if len(apps) == 0 {
		apps = append(apps, types.AppPackage{
			Name:      appName,
			Version:   "",
			RemoteDir: appName,
			Selected:  true,
		})
	}

	return apps
}

func (s *Scanner) processJarFileFromDir(jarPath, appDir, appName string) *types.AppPackage {
	filename := filepath.Base(jarPath)
	name, version := extractAppNameAndVersion(filename)
	configFiles := s.findConfigFiles(appDir, name)

	return &types.AppPackage{
		Name:         name,
		JarFileName:  filename,
		LocalJarPath: jarPath,
		ConfigFiles:  configFiles,
		RemoteDir:    appName,
		Version:      version,
		Selected:     true,
	}
}

func (s *Scanner) ScanInfraDir(dir string) ([]types.MiddlewareInstance, error) {
	var middlewares []types.MiddlewareInstance

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return middlewares, fmt.Errorf("directory does not exist: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return middlewares, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		mwDir := filepath.Join(dir, entry.Name())
		mw := types.MiddlewareInstance{
			Type:      types.MiddlewareType(entry.Name()),
			RemoteDir: entry.Name(),
			Selected:  true,
		}

		if info, _ := os.Stat(filepath.Join(mwDir, "config.yaml")); info != nil {
			mw.ConfigFile = filepath.Join(mwDir, "config.yaml")
		}

		middlewares = append(middlewares, mw)
	}

	return middlewares, nil
}

func (s *Scanner) processJarFile(jarPath string) *types.AppPackage {
	filename := filepath.Base(jarPath)
	jarDir := filepath.Dir(jarPath)

	name, version := extractAppNameAndVersion(filename)

	configFiles := s.findConfigFiles(jarDir, name)

	relPath, _ := filepath.Rel(s.scanDir, jarPath)
	remoteDir := filepath.Dir(relPath)
	if remoteDir == "." {
		remoteDir = name
	}

	return &types.AppPackage{
		Name:         name,
		JarFileName:  filename,
		LocalJarPath: jarPath,
		ConfigFiles:  configFiles,
		RemoteDir:    remoteDir,
		Version:      version,
		Selected:     true,
	}
}

func extractAppNameAndVersion(filename string) (string, string) {
	if match := jarRegex.FindStringSubmatch(filename); match != nil {
		return match[1], match[2]
	}

	name := strings.TrimSuffix(filename, ".jar")
	return name, "unknown"
}

func (s *Scanner) findConfigFiles(appDir string, appName string) []string {
	var configs []string

	configPatterns := []string{
		"application.yml",
		"application.yaml",
		"application.properties",
		"bootstrap.yml",
		"bootstrap.yaml",
		"bootstrap.properties",
	}

	for _, pattern := range configPatterns {
		path := filepath.Join(appDir, pattern)
		if _, err := os.Stat(path); err == nil {
			configs = append(configs, path)
		}
	}

	configDir := filepath.Join(appDir, "config")
	if info, err := os.Stat(configDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(configDir)
		for _, entry := range entries {
			if !entry.IsDir() {
				configs = append(configs, filepath.Join(configDir, entry.Name()))
			}
		}
	}

	return configs
}
