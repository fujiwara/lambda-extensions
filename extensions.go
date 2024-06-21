package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
)

type Event string

const (
	EventInvoke          = "INVOKE"
	EventShutdown        = "SHUTDOWN"
	DefaultTelemetryPort = 8080

	lambdaExtensionNameHeader       = "Lambda-Extension-Name"
	lambdaExtensionIdentifierHeader = "Lambda-Extension-Identifier"
)

var (
	DefaultTelemetrySubscription = TelemetrySubscription{
		SchemaVersion: "2022-12-13",
		Types:         []string{"function", "platform"},
		Buffering: TelemetryBuffering{
			MaxItems:  500,
			MaxBytes:  1024 * 1024,
			TimeoutMs: 1000,
		},
		Destination: TelemetryDestination{
			Protocol: "HTTP",
			URI:      fmt.Sprintf("http://sandbox.localdomain:%d", DefaultTelemetryPort),
		},
	}

	lambdaExtensionAPIEndpoint string
	lambdaTelemetryAPIEndpoint string
)

func init() {
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") == "" {
		panic("AWS_LAMBDA_RUNTIME_API is not set")
	}
	lambdaExtensionAPIEndpoint = "http://" + os.Getenv("AWS_LAMBDA_RUNTIME_API") + "/2020-01-01/extension"
	lambdaTelemetryAPIEndpoint = "http://" + os.Getenv("AWS_LAMBDA_RUNTIME_API") + "/2022-07-01/telemetry"
}

// Client is a client for Lambda Extensions API
type Client struct {
	Name             string // Name is the name of the extension
	CallbackInvoke   func(context.Context, *InvokeEventResponse) error
	CallbackShutdown func(context.Context, *ShutdownEventResponse) error

	extensionId string
	client      *http.Client
}

// NewClient creates a new client for Lambda Extensions API
func NewClient(ctx context.Context, name string) *Client {
	return &Client{
		Name:   name,
		client: http.DefaultClient,
	}
}

type registerPayload struct {
	Events []Event `json:"events"`
}

// Register registers the extension to the Lambda extension API
func (c *Client) Register(ctx context.Context, events []Event) error {
	u := fmt.Sprintf("%s/register", lambdaExtensionAPIEndpoint)
	b, _ := json.Marshal(registerPayload{Events: events})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	req.Header.Set(lambdaExtensionNameHeader, c.Name)
	slog.InfoContext(ctx, "registering extension", "url", u, "name", c.Name, "headers", req.Header, "events", events)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register extension: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode register response: %w", err)
	}
	slog.InfoContext(ctx, "register status", "status", resp.Status, "response", result)

	c.extensionId = resp.Header.Get(lambdaExtensionIdentifierHeader)
	if c.extensionId == "" {
		return fmt.Errorf("extension identifier is empty: %d %v", resp.StatusCode, resp.Header)
	}
	slog.InfoContext(ctx, "extension registered", "extension_id", c.extensionId)
	return nil
}

func (c *Client) fetchNextEvent(ctx context.Context) (*EventResponse, error) {
	u := fmt.Sprintf("%s/event/next", lambdaExtensionAPIEndpoint)
	slog.DebugContext(ctx, "getting next event", "url", u, "extension_id", c.extensionId)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set(lambdaExtensionIdentifierHeader, c.extensionId)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get next event: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get next event: %d", resp.StatusCode)
	}
	var ev EventResponse
	if err := json.NewDecoder(resp.Body).Decode(&ev); err != nil {
		return nil, fmt.Errorf("failed to decode event response: %w", err)
	}
	return &ev, nil
}

// Run runs the extension client
func (c *Client) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		ev, err := c.fetchNextEvent(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to fetch next event", "error", err)
			continue
		}
		if ev.Invoke != nil {
			slog.DebugContext(ctx, "invoke event received")
			if c.CallbackInvoke != nil {
				if err := c.CallbackInvoke(ctx, ev.Invoke); err != nil {
					slog.ErrorContext(ctx, "invoke callback failed", "error", err)
				}
			} else {
				slog.WarnContext(ctx, "invoke callback is not set")
			}
		} else if ev.Shutdown != nil {
			slog.DebugContext(ctx, "shutdown event received. shutting down extension")
			if c.CallbackShutdown != nil {
				if err := c.CallbackShutdown(ctx, ev.Shutdown); err != nil {
					slog.ErrorContext(ctx, "shutdown callback failed", "error", err)
					return fmt.Errorf("shutdown callback failed: %w", err)
				}
			} else {
				slog.WarnContext(ctx, "shutdown callback is not set")
			}
			return nil
		} else {
			slog.WarnContext(ctx, "unknown event received")
		}
	}
}

// SubscribeTelemetry subscribes to the telemetry API
func (c *Client) SubscribeTelemetry(ctx context.Context, subscription *TelemetrySubscription) error {
	u := lambdaTelemetryAPIEndpoint
	if subscription == nil {
		subscription = NewDefaultTelemetrySubscription()
	}
	s, _ := json.Marshal(subscription)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(s))
	req.Header.Set(lambdaExtensionNameHeader, c.Name)
	req.Header.Set(lambdaExtensionIdentifierHeader, c.extensionId)
	slog.InfoContext(ctx, "subscribing telemetry API", "url", u, "name", c.Name, "headers", req.Header, "payload", string(s))

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register extension: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to subscribe telemetry API: %d %s", resp.StatusCode, string(b))
	} else {
		slog.InfoContext(ctx, "subscribed telemetry API", "status", resp.Status, "response", string(b))
	}
	return nil
}

type TelemetrySubscription struct {
	SchemaVersion string               `json:"schemaVersion"`
	Types         []string             `json:"types"`
	Buffering     TelemetryBuffering   `json:"buffering"`
	Destination   TelemetryDestination `json:"destination"`
}

type TelemetryBuffering struct {
	MaxItems  int `json:"maxItems"`
	MaxBytes  int `json:"maxBytes"`
	TimeoutMs int `json:"timeoutMs"`
}

type TelemetryDestination struct {
	Protocol string `json:"protocol"`
	URI      string `json:"URI"`
}

func NewDefaultTelemetrySubscription() *TelemetrySubscription {
	s := DefaultTelemetrySubscription
	return &s
}

/*
{
   "schemaVersion": "2022-12-13",
   "types": [
        "platform",
        "function",
        "extension"
   ],
   "buffering": {
        "maxItems": 1000,
        "maxBytes": 256*1024,
        "timeoutMs": 100
   },
   "destination": {
        "protocol": "HTTP",
        "URI": "http://sandbox.localdomain:8080"
   }
}*/

/*
{
    "eventType": "INVOKE",
    "deadlineMs": 676051,
    "requestId": "3da1f2dc-3222-475e-9205-e2e6c6318895",
    "invokedFunctionArn": "arn:aws:lambda:us-east-1:123456789012:function:ExtensionTest",
    "tracing": {
        "type": "X-Amzn-Trace-Id",
        "value": "Root=1-5f35ae12-0c0fec141ab77a00bc047aa2;Parent=2be948a625588e32;Sampled=1"
    }
 }
*/

type InvokeEventResponse struct {
	EventType          string `json:"eventType"`
	DeadlineMs         int    `json:"deadlineMs"`
	RequestID          string `json:"requestId"`
	InvokedFunctionArn string `json:"invokedFunctionArn"`
	Tracing            struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"tracing"`
}

/*
{
  "eventType": "SHUTDOWN",
  "shutdownReason": "reason for shutdown",
  "deadlineMs": "the time and date that the function times out in Unix time milliseconds"
}
*/

type ShutdownEventResponse struct {
	EventType      string `json:"eventType"`
	DeadlineMs     int    `json:"deadlineMs"`
	ShutdownReason string `json:"shutdownReason"`
}

type CommonResponse struct {
	EventType string `json:"eventType"`
}

type EventResponse struct {
	Invoke   *InvokeEventResponse
	Shutdown *ShutdownEventResponse
}

func (r *EventResponse) UnmarshalJSON(data []byte) error {
	var common CommonResponse
	if err := json.Unmarshal(data, &common); err != nil {
		return err
	}
	switch common.EventType {
	case EventInvoke:
		r.Invoke = &InvokeEventResponse{}
		return json.Unmarshal(data, r.Invoke)
	case EventShutdown:
		r.Shutdown = &ShutdownEventResponse{}
		return json.Unmarshal(data, r.Shutdown)
	default:
		return fmt.Errorf("unknown event type: %s", common.EventType)
	}
}
