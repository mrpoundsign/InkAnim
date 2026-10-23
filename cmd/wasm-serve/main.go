package main

import (
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"time"
)

func main() {
	port := flag.String("port", "8080", "Port to serve on")
	dir := flag.String("dir", "build/web", "Directory to serve")
	flag.Parse()

	_ = mime.AddExtensionType(".wasm", "application/wasm")
	fs := http.FileServer(http.Dir(*dir))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "must-revalidate, max-age=0")
		fs.ServeHTTP(w, r)
	})

	fmt.Printf("==> Serving InkAnim Web:\n")
	fmt.Printf("    Landing Page:     http://localhost:%s/\n", *port)
	fmt.Printf("    Interactive Demo: http://localhost:%s/demo/\n\n", *port)
	fmt.Println("Press Ctrl+C to stop.")

	srv := &http.Server{
		Addr:              "127.0.0.1:" + *port,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
