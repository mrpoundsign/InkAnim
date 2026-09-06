package main

import (
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
)

func main() {
	port := flag.String("port", "8080", "Port to serve on")
	dir := flag.String("dir", "build/gh-pages", "Directory to serve")
	flag.Parse()

	_ = mime.AddExtensionType(".wasm", "application/wasm")
	fs := http.FileServer(http.Dir(*dir))
	http.Handle("/", fs)

	fmt.Printf("==> Serving InkAnim Web:\n")
	fmt.Printf("    Landing Page: http://localhost:%s/\n", *port)
	fmt.Printf("    Live Demo:    http://localhost:%s/demo/\n\n", *port)
	fmt.Println("Press Ctrl+C to stop.")

	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
