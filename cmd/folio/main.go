package main

import (
	"fmt"
	"os"

	"folio/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Folio:", err)
		os.Exit(1)
	}
}
