package main

import (
	"log"

	_ "github.com/psds-microservice/infra" // для go mod vendor (proto-build)
	"github.com/psds-microservice/ticket-service/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
