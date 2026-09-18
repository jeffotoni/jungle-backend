package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed index.html api.yaml
var staticFiles embed.FS

func main() {
	addr := os.Getenv("SWAGGER_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           http.FileServer(http.FS(staticFiles)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Fatal(server.ListenAndServe())
}
