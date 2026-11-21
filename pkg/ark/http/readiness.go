package http

import (
	"context"
	"fmt"
	"time"
)

func WaitForServerReadiness(ctx context.Context, uri string) error {
	clientCfg := ClientCfg{
		BaseURI:             uri,
		DisableRedirections: true,
	}

	client, err := NewClient(&clientCfg)
	if err != nil {
		return fmt.Errorf("cannot create HTTP client: %w", err)
	}

	delay := 250 * time.Millisecond

	for {
		_, err := client.SendRequest(ctx, "GET", "", nil, nil, nil)
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return err

		case <-time.After(delay):
		}
	}
}
