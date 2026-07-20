package main

import (
	"os"

	"github.com/verba-lang/verba/internal/cli"
)

func main() {
	os.Exit(cli.New().Run(os.Args[1:]))
}
