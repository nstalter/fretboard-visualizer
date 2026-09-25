package main

import (
	"embed"
	"flag"
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
	addr := flag.String("addr", "localhost:8080", "address to listen on")
	flag.Parse()
	log.Printf("Fretboard Visualizer on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, newMux(static)))
}
