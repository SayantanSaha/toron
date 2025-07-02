package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	port := "9000" // Default port

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Log request details
		log.Printf("%s %s %s from %s\n", r.Method, r.URL.Path, r.Proto, r.RemoteAddr)
		for name, values := range r.Header {
			for _, value := range values {
				log.Printf("Header: %s: %s\n", name, value)
			}
		}
		log.Println("----")

		fmt.Fprintf(w, "<a href=\"http://localhost:9000/foo\">Foo</a>\n")

	})

	log.Printf("Server starting on port %s...\n", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
