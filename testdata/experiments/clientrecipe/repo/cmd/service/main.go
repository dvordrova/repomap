package main

import (
	"context"
	"log"

	"example.com/launchservice/internal/app"
	"example.com/launchservice/internal/config"
)

func main() {
	service, err := app.Bootstrap(config.FromEnvironment())
	if err != nil {
		log.Fatal(err)
	}
	if err := service.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
