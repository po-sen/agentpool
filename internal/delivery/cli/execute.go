package cli

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Execute runs an HTTP client command.
func Execute(ctx context.Context, command Command, stdout io.Writer) error {
	client, err := NewClient(command.Addr)
	if err != nil {
		return err
	}

	switch command.Kind {
	case CommandRun:
		return executeRun(ctx, client, command, stdout)
	case CommandWatch:
		return executeWatch(ctx, client, command, stdout)
	case CommandGet:
		return executeGet(ctx, client, command, stdout)
	case CommandList:
		return executeList(ctx, client, command, stdout)
	case CommandCancel:
		return executeCancel(ctx, client, command, stdout)
	case CommandArtifacts:
		return executeArtifacts(ctx, client, command, stdout)
	case CommandArtifact:
		return executeArtifact(ctx, client, command, stdout)
	default:
		return fmt.Errorf("unsupported HTTP client command: %s", command.Kind)
	}
}

func executeRun(ctx context.Context, client *Client, command Command, stdout io.Writer) error {
	if command.Run.Watch && (!command.Run.Wait || command.Output.JSON) {
		return ErrUsage
	}
	created, err := client.CreateRun(ctx, CreateRunRequest{
		Prompt:   command.Run.Prompt,
		Files:    command.Run.Files,
		Dirs:     command.Run.Dirs,
		Archives: command.Run.Archives,
	})
	if err != nil {
		return err
	}
	response := created
	if command.Run.Wait {
		waitCtx, cancel := waitContext(ctx, command.Run.Timeout)
		defer cancel()
		if command.Run.Watch {
			response, err = watchRun(waitCtx, client, created.ID, &created, command.Run.PollInterval, stdout, command.Output)
		} else {
			response, err = client.WaitRun(waitCtx, created.ID, command.Run.PollInterval)
		}
		if err != nil {
			return err
		}
	}
	if command.Run.Watch {
		return nil
	}

	return WriteRunOutput(stdout, response, command.Output)
}

func executeWatch(ctx context.Context, client *Client, command Command, stdout io.Writer) error {
	if command.Output.JSON {
		return ErrUsage
	}
	waitCtx, cancel := waitContext(ctx, command.Run.Timeout)
	defer cancel()
	_, err := watchRun(waitCtx, client, command.RunID, nil, command.Run.PollInterval, stdout, command.Output)

	return err
}

func waitContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
}

func executeGet(ctx context.Context, client *Client, command Command, stdout io.Writer) error {
	response, err := client.GetRun(ctx, command.RunID)
	if err != nil {
		return err
	}

	return WriteRunOutput(stdout, response, command.Output)
}

func executeList(ctx context.Context, client *Client, command Command, stdout io.Writer) error {
	responses, err := client.ListRuns(ctx)
	if err != nil {
		return err
	}

	return WriteRunsOutput(stdout, responses, command.Output)
}

func executeCancel(ctx context.Context, client *Client, command Command, stdout io.Writer) error {
	response, err := client.CancelRun(ctx, command.RunID)
	if err != nil {
		return err
	}

	return WriteRunOutput(stdout, response, command.Output)
}

func executeArtifacts(ctx context.Context, client *Client, command Command, stdout io.Writer) error {
	response, err := client.ListArtifacts(ctx, command.RunID)
	if err != nil {
		return err
	}

	return WriteArtifactsOutput(stdout, response, command.Output)
}

func executeArtifact(ctx context.Context, client *Client, command Command, stdout io.Writer) error {
	artifact, err := client.GetArtifact(ctx, command.RunID, command.Path)
	if err != nil {
		return err
	}
	_, err = stdout.Write(artifact.Content)

	return err
}
