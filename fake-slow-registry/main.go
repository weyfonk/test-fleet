package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sync/semaphore"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

var listenAddr = flag.String("listenAddr", ":8080", "http service address")
var indexConcurrency = flag.Int64("max-index-concurrency", 1, "maximum number of concurrent index.yaml requests")
var simulatedIndexDuration = flag.Duration("simulate-index-duration", 0, "simulate wait time for generating index.yaml")

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancel()
		<-c
		os.Exit(1)
	}()

	if err := run(ctx); err != nil {
		log.Fatalf("%+v\n", err)
	}
}

func run(ctx context.Context) error {
	router, err := newRouter(http.DefaultServeMux)
	if err != nil {
		return err
	}
	s := &http.Server{Addr: *listenAddr, Handler: router}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		log.Printf("Listening on %s\n", s.Addr)
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s\n", err)
		}
	}()
	<-ctx.Done()

	shutdownCtx, cancelShutdownCtx := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelShutdownCtx()

	return s.Shutdown(shutdownCtx)
}

func newRouter(mux *http.ServeMux) (http.Handler, error) {
	sem := semaphore.NewWeighted(*indexConcurrency)
	index := repo.NewIndexFile()
	if err := index.MustAdd(&chart.Metadata{
		APIVersion: "v2",
		Version:    "0.0.0",
		Name:       "guestbook",
		AppVersion: "0.1.0",
	}, "charts/guestbook-0.0.0.tgz", "", "a60ac9484e3c20c298cbc83052ffdfb5c25d27843685e38cfec66221c43cb491"); err != nil {
		return nil, err
	}
	if err := index.MustAdd(&chart.Metadata{
		APIVersion: "v2",
		Version:    "0.1.0",
		Name:       "config-chart",
		AppVersion: "0.1.0",
	}, "charts/config-chart-0.1.0.tgz", "", "a60ac9484e3c20c298cbc83052ffdfb5c25d27843685e38cfec66221c43cb491"); err != nil {
		return nil, err
	}
	indexBytes, err := verifyIndex(index)
	if err != nil {
		return nil, err
	}
	var count int32
	mux.HandleFunc("/index.yaml", func(rw http.ResponseWriter, req *http.Request) {
		if err := sem.Acquire(req.Context(), 1); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
		defer sem.Release(1)

		n := atomic.AddInt32(&count, 1)
		log.Printf("serving index.yaml for %dth time...\n", n)

		if *simulatedIndexDuration > 0 {
			select {
			case <-req.Context().Done():
				http.Error(rw, "context canceled", http.StatusInternalServerError)
				return
			case <-time.After(*simulatedIndexDuration):
			}
		}

		if _, err := rw.Write(indexBytes); err != nil {
			log.Println("failed encoding index.yaml:", err)
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	mux.Handle("/charts/", http.FileServer(http.Dir("")))
	return mux, nil
}

func verifyIndex(index *repo.IndexFile) ([]byte, error) {
	bytes, err := yaml.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal index.yaml: %v", err)
	}

	index = &repo.IndexFile{}
	if err := yaml.Unmarshal(bytes, index); err != nil {
		return nil, fmt.Errorf("failed to unmarshal index.yaml: %v", err)
	}
	if _, err := index.Get("config-chart", "0.1.0"); err != nil {
		return nil, fmt.Errorf("error looking for chart in index.yaml: %w", err)
	}
	return bytes, nil
}
