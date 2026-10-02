package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed web
var webFS embed.FS

func main() {
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	addr := flag.String("addr", "localhost:8080", "address to listen on")
	dbPath := flag.String("db", "fretboard.db", "SQLite database for accounts' songs (used only when sign-in is configured)")
	devUser := flag.String("dev-user", "", "sign everyone in as this email, without Cognito (local development only)")
	flag.Parse()
	mux := newMux(static)
	if err := setupAccounts(context.Background(), mux, *addr, *dbPath, *devUser); err != nil {
		log.Fatal(err)
	}
	log.Printf("Fretboard Visualizer on http://%s", *addr)
	// Timeouts keep slow or stalled clients from holding connections open on a public site.
	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	log.Fatal(srv.ListenAndServe())
}
