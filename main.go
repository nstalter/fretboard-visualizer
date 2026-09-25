package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed web
var webFS embed.FS

func main() {
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	addr := "localhost:8080"
	log.Printf("Fretboard Visualizer on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, newMux(static)))
}
