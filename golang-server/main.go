package main

import (
	"log"

	"exiro.ai/application"
)

func main() {
	if err := application.Run(); err != nil {
		log.Fatal(err)
	}
}
