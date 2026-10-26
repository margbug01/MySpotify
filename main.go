package main

import (
	"log"

	"netease-music-mygo/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
