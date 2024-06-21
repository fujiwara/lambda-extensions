# lambda-extensions

lambda-extensions is a library for [AWS Lambda Extensions API](https://docs.aws.amazon.com/lambda/latest/dg/runtimes-extensions-api.html).

## Usage

### Register an external extension

```go
package main

import (
    "context"
    "log"

    extensions "github.com/fujiwara/lambda-extensions"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    client := extensions.NewClient("my-extension")
    client.CallbackInvoke = func (ctx context.Context, event *extensions.InvokeEvent) error {
        log.Printf("invoke event: %v", event)
        // do something on invoke event
        return nil
    }
    client.CallbackShutdown = func (ctx context.Context, event *extensions.ShutdownEvent) error {
        log.Printf("shutdown event: %v", event)
        // do something on shutdown event
        cancel()
        return nil
    }
    if err := client.Register(ctx); err != nil {
        log.Fatal(err)
    }
    if err := client.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

### Subscribe to Telemetry API

```go
package main

import (
    "context"
    "log"

    extensions "github.com/fujiwara/lambda-extensions"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go func () {
       // run http server listening on 9999 for receiving telemetry data
    }()

    client := extensions.NewClient("my-telemetry")
    client.CallbackShutdown = func (ctx context.Context, event *extensions.ShutdownEvent) error {
        log.Printf("shutdown event: %v", event)
        // do something
        stopHttpServer() // stop your http server
        return nil
    }
    if err := client.Register(ctx); err != nil {
        log.Fatal(err)
    }
    sub := &extensions.TelemetrySubscription{
		SchemaVersion: "2022-12-13",
		Types:         []string{"function", "platform"},
		Buffering: extensions.TelemetryBuffering{
			MaxItems:  500,
			MaxBytes:  1024 * 1024,
			TimeoutMs: 1000,
		},
		Destination: extensions.TelemetryDestination{
			Protocol: "HTTP",
			URI:      "http://sandbox.localdomain:9999",
		},
	}
    if err := client.SubscribeTelemetry(ctx, sub); err != nil {
        log.Fatal(err)
    }
    if err := client.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## LICENSE

MIT

## Author

fujiwara
