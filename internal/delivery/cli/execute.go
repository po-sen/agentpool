package cli

import (
	"context"
	"fmt"
	"io"
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
	case CommandGet:
		return executeGet(ctx, client, command, stdout)
	case CommandList:
		return executeList(ctx, client, command, stdout)
	case CommandCancel:
		return executeCancel(ctx, client, command, stdout)
	default:
		return fmt.Errorf("unsupported HTTP client command: %s", command.Kind)
	}
}

func executeRun(ctx context.Context, client *Client, command Command, stdout io.Writer) error {
	created, err := client.CreateRun(ctx, CreateRunRequest{
		Prompt: command.Run.Prompt,
		Files:  command.Run.Files,
	})
	if err != nil {
		return err
	}
	response := created
	if command.Run.Wait {
		waitCtx, cancel := context.WithTimeout(ctx, command.Run.Timeout)
		defer cancel()
		response, err = client.WaitRun(waitCtx, created.ID, command.Run.PollInterval)
		if err != nil {
			return err
		}
	}

	return WriteRunOutput(stdout, response, command.Output)
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
