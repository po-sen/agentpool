package bootstrap

import (
	"context"
	"fmt"
	"io"

	"github.com/po-sen/agentpool/internal/adapters/inbound/httpapi"
	"github.com/po-sen/agentpool/internal/adapters/outbound/agent"
	"github.com/po-sen/agentpool/internal/adapters/outbound/events"
	"github.com/po-sen/agentpool/internal/adapters/outbound/git"
	"github.com/po-sen/agentpool/internal/adapters/outbound/ids"
	"github.com/po-sen/agentpool/internal/adapters/outbound/memory"
	"github.com/po-sen/agentpool/internal/adapters/outbound/policy"
	"github.com/po-sen/agentpool/internal/adapters/outbound/sandbox"
	"github.com/po-sen/agentpool/internal/adapters/outbound/secrets"
	"github.com/po-sen/agentpool/internal/application/command"
	"github.com/po-sen/agentpool/internal/application/query"
	"github.com/po-sen/agentpool/internal/application/workflow"
	"github.com/po-sen/agentpool/internal/config"
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
	eventPublisher := events.NewNoopPublisher()
	idGenerator := ids.NewCryptoGenerator()

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

	sandboxProvider := sandbox.NewNoopProvider()
	agentExecutor := agent.NewNoopExecutor()
	gitProvider := git.NewNoopProvider()
	policyDecision := policy.NewAllowAllDecision()
	secretBroker := secrets.NewNoopBroker()

	worker := workflow.NewWorker(workflow.WorkerDependencies{
		Queue:      runQueue,
		Repo:       runRepo,
		StateStore: runRepo,
		Events:     eventPublisher,
		Sandbox:    sandboxProvider,
		Agent:      agentExecutor,
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
