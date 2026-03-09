package main

import (
	"log"

	"github.com/rustic-ai/forge/forge-go/command"
)

func main() {
	if err := command.Execute(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
