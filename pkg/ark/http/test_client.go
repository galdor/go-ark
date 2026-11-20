package http

import (
	"context"
	"net/http"
	"testing"
)

type TestClient struct {
	*Client

	T testing.TB
}

func NewTestClient(t testing.TB, cfg *ClientCfg) *TestClient {
	c, err := NewClient(cfg)
	if err != nil {
		t.Errorf("cannot create HTTP client: %v", err)
		t.FailNow()
	}

	return &TestClient{Client: c, T: t}
}

func (c *TestClient) SendRequest(
	method, uri string, header http.Header, reqBody, resBody any,
) *http.Response {
	ctx := context.Background()

	res, err := c.Client.SendRequest(ctx, method, uri, header, reqBody, resBody)
	if err != nil {
		c.T.Errorf("cannot send HTTP request: %v", err)
		c.T.FailNow()
	}

	return res
}
