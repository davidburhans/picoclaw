package commands

import (
	"context"
	"fmt"
)

func newCommand() Definition {
	return Definition{
		Name:        "new",
		Aliases:     []string{"rotate"},
		Description: "Start a new session (rotates conversation history)",
		Usage:       "/new",
		Handler: func(ctx context.Context, req Request, rt *Runtime) error {
			if rt.RotateSession == nil {
				return req.Reply("Session rotation not available.")
			}

			if err := rt.RotateSession(ctx, req.SessionKey); err != nil {
				return req.Reply(fmt.Sprintf("Failed to rotate session: %v", err))
			}

			return req.Reply("Started a new session.")
		},
	}
}
