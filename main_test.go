package main

import (
	"log"
	"testing"
)

func TestParseEndpoints(t *testing.T) {
	f := "https://prometheus./-/healthy,https://alertmanager/-/healthy;addr=::1"

	eps, err := parseEndpoints(f)
	if err != nil {
		log.Fatal(err)
	}

	if eps[0].url.String() != "https://prometheus./-/healthy" {
		t.Errorf("want %s got %s", "https://prometheus./-/healthy", eps[0].url.String())
	}

	if eps[1].url.String() != "https://alertmanager/-/healthy" {
		t.Errorf("want %s got %s", "https://alertmanager/-/healthy", eps[0].url.String())
	}
	if eps[1].addr != "::1" {
		t.Errorf("want %s got %s", "::1", eps[1].addr)
	}
}
