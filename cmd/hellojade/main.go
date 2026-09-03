// Command hellojade is the command-line client for the hellojade Partner
// Intake API.
package main

import (
	"os"

	"github.com/hellojade-ai/leads-cli/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}
