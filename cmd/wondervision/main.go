package main

import (
	"fmt"
	"os"

	"github.com/jewei/wondervision/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "WonderVision:", err)
		os.Exit(1)
	}
}
