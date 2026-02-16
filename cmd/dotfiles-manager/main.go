package main

import (
	"os"

	"github.com/shpoont/dotfiles-manager/internal/app"
)

var osExit = os.Exit

func run() int {
	return app.Execute()
}

func main() {
	osExit(run())
}
