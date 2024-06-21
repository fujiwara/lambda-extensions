package main

import (
	"log/slog"
	"net/http"
	"os"

	extensions "github.com/fujiwara/lambda-extensions"
)

func main() {
	port := os.Args[1]
	h := extensions.MockExtensionAPIHandler()
	slog.Info("listening", "port", port)
	http.ListenAndServe(":"+port, h)
}
