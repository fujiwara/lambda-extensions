package extensions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func MockExtensionAPIHandler() http.Handler {
	mux := http.NewServeMux()
	invokeCh := make(chan struct{})
	shutdownCh := make(chan struct{})
	mux.HandleFunc("/2020-01-01/extension/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(lambdaExtensionIdentifierHeader, "0000-0000-0000-0000")
		ev := registerPayload{}
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slog.Info("registering", "events", ev)
		for _, e := range ev.Events {
			switch e {
			case Invoke:
				slog.Info("registering invoke event")
				go func() {
					tk := time.NewTicker(1 * time.Second)
					for range tk.C {
						slog.Info("invoke event")
						invokeCh <- struct{}{}
					}
				}()
			case Shutdown:
				slog.Info("registering shutdown event")
				go func() {
					timer := time.NewTimer(5 * time.Second)
					<-timer.C
					slog.Info("shutdown event")
					shutdownCh <- struct{}{}
				}()
			}
		}
		fmt.Fprintf(w, `{"functionName": "helloWorld","functionVersion": "$LATEST","handler": "handler"}`)
	})
	mux.HandleFunc("/2020-01-01/extension/event/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		select {
		case <-invokeCh:
			fmt.Fprintf(w, `{"eventType": "INVOKE"}`)
		case <-shutdownCh:
			fmt.Fprintf(w, `{"eventType": "SHUTDOWN"}`)
		}
	})
	mux.HandleFunc("/2022-07-01/telemetry", func(w http.ResponseWriter, r *http.Request) {
		sub := TelemetrySubscription{}
		if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dest := strings.Replace(sub.Destination.URI, "sandbox.localdomain", "localhost", 1)
		go func() {
			tk := time.NewTicker(1 * time.Second)
			n := 0
			for range tk.C {
				slog.Info("sending telemetry")
				ps := []telemetryPayload{
					{Time: time.Now().Format(time.RFC3339), Type: "function", Record: fmt.Sprintf("record-%d", n)},
				}
				b, _ := json.Marshal(ps)
				resp, err := http.Post(dest, "application/json", bytes.NewReader(b))
				if err != nil {
					slog.Error("failed to send telemetry", "error", err)
					return
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				slog.Info("sent telemetry", "status", resp.Status)
			}
		}()
	})
	return mux
}

type telemetryPayload struct {
	Time   string `json:"time"`
	Type   string `json:"type"`
	Record string `json:"record"`
}
