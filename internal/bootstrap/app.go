package bootstrap

import (
	"context"
	"fmt"
	"io"

	applicationagent "github.com/po-sen/agentpool/internal/application/agent"
	"github.com/po-sen/agentpool/internal/application/command"
	"github.com/po-sen/agentpool/internal/application/port/outbound"
	"github.com/po-sen/agentpool/internal/application/query"
	"github.com/po-sen/agentpool/internal/application/workflow"
	"github.com/po-sen/agentpool/internal/config"
	"github.com/po-sen/agentpool/internal/delivery/httpapi"
	eventnoop "github.com/po-sen/agentpool/internal/infrastructure/event/noop"
	"github.com/po-sen/agentpool/internal/infrastructure/id/crypto"
	"github.com/po-sen/agentpool/internal/infrastructure/llm/anthropic"
	"github.com/po-sen/agentpool/internal/infrastructure/llm/gemini"
	llmnoop "github.com/po-sen/agentpool/internal/infrastructure/llm/noop"
	"github.com/po-sen/agentpool/internal/infrastructure/llm/openai"
	openaicompatible "github.com/po-sen/agentpool/internal/infrastructure/llm/openai_compatible"
	"github.com/po-sen/agentpool/internal/infrastructure/persistence/memory"
	"github.com/po-sen/agentpool/internal/infrastructure/policy/allowall"
	sandboxnoop "github.com/po-sen/agentpool/internal/infrastructure/sandbox/noop"
	secretnoop "github.com/po-sen/agentpool/internal/infrastructure/secret/noop"
	"github.com/po-sen/agentpool/internal/infrastructure/tool/builtin"
	"github.com/po-sen/agentpool/internal/infrastructure/tool/composite"
	"github.com/po-sen/agentpool/internal/infrastructure/tool/workspace"
	workspacenoop "github.com/po-sen/agentpool/internal/infrastructure/workspace/noop"
	"github.com/po-sen/agentpool/internal/runtime/httpserver"
	"github.com/po-sen/agentpool/internal/runtime/logger"
)

// App contains the wired application graph.
type App struct {
	config         config.Config
	server         *httpserver.Server
	worker         *workflow.Worker
	logger         *logger.Logger
	runRepo        *memory.RunRepository
	runQueue       *memory.RunQueue
	eventPublisher outbound.EventPublisher
}

// New wires the AgentPool application.
func New(version string, logOutput io.Writer) (*App, error) {
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

	router := httpapi.NewRouter(httpapi.Dependencies{
		CreateRun: createRunHandler,
		GetRun:    getRunHandler,
		ListRuns:  listRunsHandler,
		CancelRun: cancelRunHandler,
	})

	return &App{
		config:         cfg,
		server:         httpserver.New(cfg.HTTPAddr, router, log),
		logger:         log,
		runRepo:        runRepo,
		runQueue:       runQueue,
		eventPublisher: eventPublisher,
	}, nil
}

// RunServer starts the HTTP server.
func (a *App) RunServer(ctx context.Context) error {
	return a.server.Run(ctx)
}

// RunWorker starts the worker process.
func (a *App) RunWorker(ctx context.Context) error {
	worker, err := a.workerInstance()
	if err != nil {
		return err
	}

	return worker.Run(ctx)
}

// RunDev starts the HTTP API and an embedded worker in one process.
func (a *App) RunDev(ctx context.Context) error {
	worker, err := a.workerInstance()
	if err != nil {
		return err
	}

	a.logger.Infof("starting dev mode with in-memory server and worker")

	go func() {
		if err := worker.Run(ctx); err != nil {
			a.logger.Errorf("worker stopped: %v", err)
		}
	}()

	return a.server.Run(ctx)
}

// Version returns the application version line.
func (a *App) Version() string {
	return fmt.Sprintf("agentpool %s", a.config.Version)
}

func (a *App) workerInstance() (*workflow.Worker, error) {
	if a.worker != nil {
		return a.worker, nil
	}

	sandboxProvider := sandboxnoop.NewProvider()
	modelClient, err := newModelClient(a.config.LLM)
	if err != nil {
		return nil, err
	}
	echoTools := builtin.NewRunner()
	workspaceTools := workspace.NewRunner(workspace.Config{})
	toolRunner, err := composite.NewRunner(echoTools, workspaceTools)
	if err != nil {
		return nil, err
	}
	agentRunner := applicationagent.NewRunner(
		modelClient,
		toolRunner,
		applicationagent.WithMaxTurns(a.config.Agent.MaxTurns),
	)
	workspaceProvider := workspacenoop.NewProvider()
	policyDecision := allowall.NewDecision()
	secretBroker := secretnoop.NewBroker()

	a.worker = workflow.NewWorker(workflow.WorkerDependencies{
		Queue:      a.runQueue,
		Repo:       a.runRepo,
		StateStore: a.runRepo,
		Events:     a.eventPublisher,
		Sandbox:    sandboxProvider,
		Agent:      agentRunner,
		Workspace:  workspaceProvider,
		Policy:     policyDecision,
		Secrets:    secretBroker,
	})

	return a.worker, nil
}

func newModelClient(cfg config.LLMConfig) (outbound.ModelClient, error) {
	switch cfg.Provider {
	case config.ModelProviderNoop:
		return llmnoop.NewClient(), nil
	case config.ModelProviderOpenAICompatible:
		return openaicompatible.NewClient(openaicompatible.Config{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			APIKey:  cfg.APIKey,
			Timeout: cfg.Timeout,
		})
	case config.ModelProviderOpenAI:
		return openai.NewClient(openai.Config{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			APIKey:  cfg.APIKey,
			Timeout: cfg.Timeout,
		})
	case config.ModelProviderAnthropic:
		return anthropic.NewClient(anthropic.Config{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			APIKey:  cfg.APIKey,
			Timeout: cfg.Timeout,
		})
	case config.ModelProviderGemini:
		return gemini.NewClient(gemini.Config{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			APIKey:  cfg.APIKey,
			Timeout: cfg.Timeout,
		})
	default:
		return nil, fmt.Errorf("unsupported model provider %q", cfg.Provider)
	}
}
