package main

import (
	"log"

	"vinyl/internal/app"
)

func main() {
	setAppIdentity()
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
