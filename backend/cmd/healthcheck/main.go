package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	client := &http.Client{Timeout: 3 * time.Second}
	addr := os.Getenv("YN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid YN_ADDR")
		os.Exit(1)
	}
	response, err := client.Get("http://127.0.0.1:" + port + "/readyz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "health endpoint returned %s\n", response.Status)
		os.Exit(1)
	}
}
