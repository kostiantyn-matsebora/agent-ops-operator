package main

import (
	"log"
	"net/http"
	"os"
)

// Environment: LISTEN_ADDR (default ":8081").
func main() {
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8081"
	}
	log.Printf("FAKE TELEGRAM BOT API — a test double, never a product component (listen=%s)", listen)
	if err := http.ListenAndServe(listen, New().Handler()); err != nil {
		log.Fatal(err)
	}
}
