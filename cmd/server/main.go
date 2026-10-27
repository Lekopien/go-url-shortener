// Command server runs the URL shortener.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Lekopien/go-url-shortener/internal/shortener"
)

func main() {
	addr := flag.String("addr", envOr("ADDR", ":8080"), "listen address")
	db := flag.String("db", envOr("DB_FILE", "links.json"), "JSON file for links (empty = memory only)")
	base := flag.String("base", envOr("BASE_URL", "http://localhost:8080"), "public base URL for short links")
	flag.Parse()

	store, err := shortener.NewStore(*db)
	if err != nil {
		log.Fatalf("load store: %v", err)
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           (&shortener.Server{Store: store, BaseURL: *base}).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Print("bye")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
