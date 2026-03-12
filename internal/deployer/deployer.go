package deployer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"turbod/internal/middleware"
	"turbod/internal/sshclient"
	"turbod/pkg/types"
	"turbod/templates"
)

type AppDeployer struct {
	client *sshclient.SSHClient
}

func NewAppDeployer(client *sshclient.SSHClient) *AppDeployer {
	return &AppDeployer{client: client}
}

func (d *AppDeployer) Deploy(app *types.AppPackage, remoteBaseDir string, logCallback func(string)) error {
	remoteDir := filepath.Join(remoteBaseDir, app.RemoteDir)

	logCallback(fmt.Sprintf("Deploying %s to %s", app.Name, remoteDir))

	cmds := []string{
		fmt.Sprintf("mkdir -p %s", remoteDir),
		fmt.Sprintf("mkdir -p %s/logs", remoteDir),
	}
	for _, cmd := range cmds {
		if _, err := d.client.Execute(cmd); err != nil {
			return fmt.Errorf("failed to create remote directory: %v", err)
		}
	}

	remoteJarPath := filepath.Join(remoteDir, app.JarFileName)
	logCallback(fmt.Sprintf("Uploading JAR: %s", app.JarFileName))
	if err := d.client.UploadFile(app.LocalJarPath, remoteJarPath); err != nil {
		return fmt.Errorf("failed to upload JAR: %v", err)
	}

	for _, configFile := range app.ConfigFiles {
		configName := filepath.Base(configFile)
		remoteConfigPath := filepath.Join(remoteDir, configName)
		logCallback(fmt.Sprintf("Uploading config: %s", configName))
		if err := d.client.UploadFile(configFile, remoteConfigPath); err != nil {
			logCallback(fmt.Sprintf("Warning: Failed to upload config %s: %v", configName, err))
		}
	}

	startScriptPath := filepath.Join(remoteDir, "start.sh")
	existingScript := d.client.RemoteFileExists(startScriptPath)
	if !existingScript {
		logCallback("Uploading start script")
		if err := d.client.UploadContent(templates.StartScriptTemplate, startScriptPath); err != nil {
			return fmt.Errorf("failed to upload start script: %v", err)
		}
		if _, err := d.client.Execute(fmt.Sprintf("chmod +x %s", startScriptPath)); err != nil {
			return fmt.Errorf("failed to set executable permission: %v", err)
		}
	} else {
		logCallback("Start script already exists, skipping")
	}

	logCallback(fmt.Sprintf("Deployment completed: %s", app.Name))
	return nil
}

func (d *AppDeployer) Start(app *types.AppPackage, remoteBaseDir string, logCallback func(string)) error {
	remoteDir := filepath.Join(remoteBaseDir, app.RemoteDir)

	logCallback(fmt.Sprintf("Starting application: %s", app.Name))

	output, err := d.client.Execute(fmt.Sprintf("cd %s && ./start.sh %s start", remoteDir, app.Name))
	if err != nil {
		logCallback(fmt.Sprintf("Error: %v", err))
		logCallback(output)
		return err
	}

	logCallback(output)
	logCallback(fmt.Sprintf("Application started: %s", app.Name))
	return nil
}

func (d *AppDeployer) Stop(app *types.AppPackage, remoteBaseDir string, logCallback func(string)) error {
	remoteDir := filepath.Join(remoteBaseDir, app.RemoteDir)

	logCallback(fmt.Sprintf("Stopping application: %s", app.Name))

	output, err := d.client.Execute(fmt.Sprintf("cd %s && ./start.sh %s stop", remoteDir, app.Name))
	if err != nil {
		logCallback(fmt.Sprintf("Error: %v", err))
		logCallback(output)
		return err
	}

	logCallback(output)
	logCallback(fmt.Sprintf("Application stopped: %s", app.Name))
	return nil
}

func (d *AppDeployer) GetStatus(app *types.AppPackage, remoteBaseDir string) (string, error) {
	remoteDir := filepath.Join(remoteBaseDir, app.RemoteDir)

	output, err := d.client.Execute(fmt.Sprintf("cd %s && ./start.sh %s status", remoteDir, app.Name))
	return strings.TrimSpace(output), err
}

type DeploymentExecutor struct {
	client  *sshclient.DeploymentClient
	maxPara int
	results chan *types.DeploymentTask
}

func NewDeploymentExecutor(maxParallel int) *DeploymentExecutor {
	return &DeploymentExecutor{
		client:  sshclient.NewDeploymentClient(),
		maxPara: maxParallel,
		results: make(chan *types.DeploymentTask, 100),
	}
}

func (e *DeploymentExecutor) DeployApps(apps []types.AppPackage, servers []types.Server, baseDir string, progressCallback func(*types.DeploymentTask), logCallback func(string)) {
	tasks := e.createTasks(apps, servers)

	sem := make(chan struct{}, e.maxPara)

	for _, task := range tasks {
		sem <- struct{}{}
		go func(t *types.DeploymentTask) {
			e.executeTask(t, baseDir, progressCallback, logCallback)
			<-sem
		}(task)
	}

	for i := 0; i < len(tasks); i++ {
		<-e.results
	}
}

func (e *DeploymentExecutor) Deploy(apps []types.AppPackage, middlewares []types.MiddlewareInstance, servers []types.Server, appsDir, middlewareDir, stagingDir string, progressCallback func(*types.DeploymentTask), logCallback func(string)) {
	tasks := e.createDeployTasks(apps, middlewares, servers)

	sem := make(chan struct{}, e.maxPara)

	for _, task := range tasks {
		sem <- struct{}{}
		go func(t *types.DeploymentTask) {
			e.executeDeployTask(t, appsDir, middlewareDir, stagingDir, progressCallback, logCallback)
			<-sem
		}(task)
	}

	for i := 0; i < len(tasks); i++ {
		<-e.results
	}
}

func (e *DeploymentExecutor) Distribute(apps []types.AppPackage, middlewares []types.MiddlewareInstance, servers []types.Server, localAppsDir, localMiddlewareDir, remoteStagingDir string, progressCallback func(*types.DeploymentTask), logCallback func(string)) {
	tasks := e.createDeployTasks(apps, middlewares, servers)

	sem := make(chan struct{}, e.maxPara)

	for _, task := range tasks {
		sem <- struct{}{}
		go func(t *types.DeploymentTask) {
			e.executeDistributeTask(t, localAppsDir, localMiddlewareDir, remoteStagingDir, progressCallback, logCallback)
			<-sem
		}(task)
	}

	for i := 0; i < len(tasks); i++ {
		<-e.results
	}
}

func (e *DeploymentExecutor) createDeployTasks(apps []types.AppPackage, middlewares []types.MiddlewareInstance, servers []types.Server) []*types.DeploymentTask {
	var tasks []*types.DeploymentTask

	for _, app := range apps {
		if !app.Selected {
			continue
		}
		for _, server := range servers {
			if !server.Selected {
				continue
			}
			task := &types.DeploymentTask{
				ID:     fmt.Sprintf("app-%s-%s", app.Name, server.Host),
				App:    &app,
				Server: &server,
				Type:   "app",
				Status: "pending",
			}
			tasks = append(tasks, task)
		}
	}

	for _, mw := range middlewares {
		for _, serverHost := range mw.TargetServers {
			var server *types.Server
			for _, s := range servers {
				if s.Host == serverHost {
					server = &s
					break
				}
			}
			if server == nil {
				continue
			}
			task := &types.DeploymentTask{
				ID:         fmt.Sprintf("mw-%s-%s", mw.Type, server.Host),
				Middleware: &mw,
				Server:     server,
				Type:       "middleware",
				Status:     "pending",
			}
			tasks = append(tasks, task)
		}
	}

	return tasks
}

func (e *DeploymentExecutor) executeDeployTask(task *types.DeploymentTask, appsDir, middlewareDir, stagingDir string, progressCallback func(*types.DeploymentTask), logCallback func(string)) {
	task.Status = "running"
	progressCallback(task)

	client, err := e.client.GetClient(task.Server)
	if err != nil {
		task.Status = "failed"
		task.Message = err.Error()
		progressCallback(task)
		e.results <- task
		return
	}

	if task.Type == "app" && task.App != nil {
		appDeployer := NewAppDeployer(client)
		logCallback(fmt.Sprintf("[%s] Deploying app: %s", task.Server.Host, task.App.Name))

		if err := appDeployer.Deploy(task.App, appsDir, func(msg string) {
			logCallback(fmt.Sprintf("[%s] %s", task.Server.Host, msg))
		}); err != nil {
			task.Status = "failed"
			task.Message = err.Error()
			progressCallback(task)
			e.results <- task
			return
		}

		if err := appDeployer.Start(task.App, appsDir, func(msg string) {
			logCallback(fmt.Sprintf("[%s] %s", task.Server.Host, msg))
		}); err != nil {
			task.Status = "failed"
			task.Message = err.Error()
			progressCallback(task)
			e.results <- task
			return
		}
	} else if task.Type == "middleware" && task.Middleware != nil {
		mwDeployer := middleware.NewMiddlewareDeployer(client)
		logCallback(fmt.Sprintf("[%s] Deploying middleware: %s", task.Server.Host, task.Middleware.Type))

		if err := mwDeployer.Deploy(task.Middleware, task.Middleware.TargetServers, func(msg string) {
			logCallback(fmt.Sprintf("[%s] %s", task.Server.Host, msg))
		}); err != nil {
			task.Status = "failed"
			task.Message = err.Error()
			progressCallback(task)
			e.results <- task
			return
		}
	}

	task.Status = "success"
	task.Message = "Deployed successfully"
	progressCallback(task)
	e.results <- task
}

func (e *DeploymentExecutor) executeDistributeTask(task *types.DeploymentTask, appsDir, middlewareDir, stagingDir string, progressCallback func(*types.DeploymentTask), logCallback func(string)) {
	task.Status = "running"
	progressCallback(task)

	client, err := e.client.GetClient(task.Server)
	if err != nil {
		task.Status = "failed"
		task.Message = err.Error()
		progressCallback(task)
		e.results <- task
		return
	}

	if task.Type == "app" && task.App != nil {
		localDir := filepath.Join(appsDir, task.App.RemoteDir)
		remoteDir := filepath.Join(stagingDir, task.App.RemoteDir)

		logCallback(fmt.Sprintf("[%s] Distributing app: %s from %s to %s", task.Server.Host, task.App.Name, localDir, remoteDir))

		if _, err := os.Stat(localDir); os.IsNotExist(err) {
			task.Status = "failed"
			task.Message = fmt.Sprintf("local directory not found: %s", localDir)
			progressCallback(task)
			e.results <- task
			return
		}

		if err := client.UploadDirectory(localDir, remoteDir); err != nil {
			task.Status = "failed"
			task.Message = err.Error()
			progressCallback(task)
			e.results <- task
			return
		}
		logCallback(fmt.Sprintf("[%s] Distributed to: %s", task.Server.Host, remoteDir))
	} else if task.Type == "middleware" && task.Middleware != nil {
		localDir := filepath.Join(middlewareDir, string(task.Middleware.Type))
		remoteDir := filepath.Join(stagingDir, string(task.Middleware.Type))

		logCallback(fmt.Sprintf("[%s] Distributing middleware: %s from %s to %s", task.Server.Host, task.Middleware.Type, localDir, remoteDir))

		if _, err := os.Stat(localDir); os.IsNotExist(err) {
			task.Status = "failed"
			task.Message = fmt.Sprintf("local directory not found: %s", localDir)
			progressCallback(task)
			e.results <- task
			return
		}

		if err := client.UploadDirectory(localDir, remoteDir); err != nil {
			task.Status = "failed"
			task.Message = err.Error()
			progressCallback(task)
			e.results <- task
			return
		}
		logCallback(fmt.Sprintf("[%s] Distributed to: %s", task.Server.Host, remoteDir))
	}

	task.Status = "success"
	task.Message = "Distributed successfully"
	progressCallback(task)
	e.results <- task
}

func (e *DeploymentExecutor) createTasks(apps []types.AppPackage, servers []types.Server) []*types.DeploymentTask {
	var tasks []*types.DeploymentTask
	for _, app := range apps {
		if !app.Selected {
			continue
		}
		for _, server := range servers {
			if !server.Selected {
				continue
			}
			task := &types.DeploymentTask{
				ID:     fmt.Sprintf("%s-%s", app.Name, server.Host),
				App:    &app,
				Server: &server,
				Type:   "app",
				Status: "pending",
			}
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func (e *DeploymentExecutor) executeTask(task *types.DeploymentTask, baseDir string, progressCallback func(*types.DeploymentTask), logCallback func(string)) {
	task.Status = "running"
	progressCallback(task)

	client, err := e.client.GetClient(task.Server)
	if err != nil {
		task.Status = "failed"
		task.Message = err.Error()
		progressCallback(task)
		e.results <- task
		return
	}

	deployer := NewAppDeployer(client)

	logCallback(fmt.Sprintf("[%s] Starting deployment of %s", task.Server.Host, task.App.Name))

	if err := deployer.Deploy(task.App, baseDir, func(msg string) {
		logCallback(fmt.Sprintf("[%s] %s", task.Server.Host, msg))
	}); err != nil {
		task.Status = "failed"
		task.Message = err.Error()
		progressCallback(task)
		e.results <- task
		return
	}

	if err := deployer.Start(task.App, baseDir, func(msg string) {
		logCallback(fmt.Sprintf("[%s] %s", task.Server.Host, msg))
	}); err != nil {
		task.Status = "failed"
		task.Message = err.Error()
		progressCallback(task)
		e.results <- task
		return
	}

	task.Status = "success"
	task.Message = "Deployed successfully"
	progressCallback(task)
	e.results <- task
}

func (e *DeploymentExecutor) Close() {
	e.client.CloseAll()
}
