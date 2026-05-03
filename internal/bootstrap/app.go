package bootstrap

import (
	"context"
	"fmt"
	"io"

	applicationagent "github.com/po-sen/agentpool/internal/application/agent"
	"github.com/po-sen/agentpool/internal/application/command"
	"github.com/po-sen/agentpool/internal/application/query"
	"github.com/po-sen/agentpool/internal/application/workflow"
	"github.com/po-sen/agentpool/internal/config"
	"github.com/po-sen/agentpool/internal/delivery/httpapi"
	eventnoop "github.com/po-sen/agentpool/internal/infrastructure/event/noop"
	gitnoop "github.com/po-sen/agentpool/internal/infrastructure/git/noop"
	"github.com/po-sen/agentpool/internal/infrastructure/id/crypto"
	llmnoop "github.com/po-sen/agentpool/internal/infrastructure/llm/noop"
	"github.com/po-sen/agentpool/internal/infrastructure/persistence/memory"
	"github.com/po-sen/agentpool/internal/infrastructure/policy/allowall"
	sandboxnoop "github.com/po-sen/agentpool/internal/infrastructure/sandbox/noop"
	secretnoop "github.com/po-sen/agentpool/internal/infrastructure/secret/noop"
	"github.com/po-sen/agentpool/internal/runtime/httpserver"
	"github.com/po-sen/agentpool/internal/runtime/logger"
)

// App contains the wired application graph.
type App struct {
	config config.Config
	server *httpserver.Server
	worker *workflow.Worker
	logger *logger.Logger
}

// New wires the AgentPool application.
func New(version string, logOutput io.Writer) *App {
	cfg := config.Load(version)
	log := logger.New(logOutput)

	runRepo := memory.NewRunRepository()
	runQueue := memory.NewRunQueue()
	eventPublisher := eventnoop.NewPublisher()
	idGenerator := crypto.NewGenerator()

	createRunHandler := command.NewCreateRunHandler(
		runRepo,
		runQueue,
		eventPublisher,
		idGenerator,
	)
	cancelRunHandler := command.NewCancelRunHandler(
		runRepo,
		runRepo,
		eventPublisher,
	)
	getRunHandler := query.NewGetRunHandler(runRepo)
	listRunsHandler := query.NewListRunsHandler(runRepo)

	sandboxProvider := sandboxnoop.NewProvider()
	modelClient := llmnoop.NewClient()
	agentRunner := applicationagent.NewRunner(modelClient)
	gitProvider := gitnoop.NewProvider()
	policyDecision := allowall.NewDecision()
	secretBroker := secretnoop.NewBroker()

	worker := workflow.NewWorker(workflow.WorkerDependencies{
		Queue:      runQueue,
		Repo:       runRepo,
		StateStore: runRepo,
		Events:     eventPublisher,
		Sandbox:    sandboxProvider,
		Agent:      agentRunner,
		Git:        gitProvider,
		Policy:     policyDecision,
		Secrets:    secretBroker,
	})

	router := httpapi.NewRouter(httpapi.Dependencies{
		CreateRun: createRunHandler,
		GetRun:    getRunHandler,
		ListRuns:  listRunsHandler,
		CancelRun: cancelRunHandler,
	})

	return &App{
		config: cfg,
		server: httpserver.New(cfg.HTTPAddr, router, log),
		worker: worker,
		logger: log,
	}
}

// RunServer starts the HTTP server.
func (a *App) RunServer(ctx context.Context) error {
	return a.server.Run(ctx)
}

// RunWorker starts the worker process.
func (a *App) RunWorker(ctx context.Context) error {
	return a.worker.Run(ctx)
}

// RunDev starts the HTTP API and an embedded worker in one process.
func (a *App) RunDev(ctx context.Context) error {
	a.logger.Infof("starting dev mode with in-memory server and worker")

	go func() {
		if err := a.worker.Run(ctx); err != nil {
			a.logger.Errorf("worker stopped: %v", err)
		}
	}()

	return a.server.Run(ctx)
}

// Version returns the application version line.
func (a *App) Version() string {
	return fmt.Sprintf("agentpool %s", a.config.Version)
}
