package extensions_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	extensions "github.com/fujiwara/lambda-extensions"
)

func TestMockAPI(t *testing.T) {
	sv := httptest.NewServer(extensions.MockExtensionAPIHandler())
	t.Log(sv.URL)
	u, _ := url.Parse(sv.URL)
	os.Setenv("AWS_LAMBDA_RUNTIME_API", u.Host)
	defer sv.Close()

	ctx := context.Background()
	c, err := extensions.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	c.CallbackInvoke = func(ctx context.Context, e *extensions.InvokeEvent) error {
		if e.EventType != extensions.Invoke {
			t.Errorf("unexpected event type on invoke: %s", e.EventType)
		}
		return nil
	}
	c.CallbackShutdown = func(ctx context.Context, e *extensions.ShutdownEvent) error {
		if e.EventType != extensions.Shutdown {
			t.Errorf("unexpected event type on shutdown: %s", e.EventType)
		}
		return nil
	}
	if err := c.Register(ctx); err != nil {
		t.Errorf("failed to register extension: %v", err)
	}
	if err := c.Run(context.Background()); err != nil {
		t.Errorf("failed to run extension client: %v", err)
	}
}
