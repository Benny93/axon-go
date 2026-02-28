// Axon - Graph-powered code intelligence engine for Go.
//
// Axon indexes Go codebases into a structural knowledge graph,
// enabling powerful code search, impact analysis, and more.
package main

import (
	"fmt"
	"os"

	"github.com/Benny93/axon-go/cmd"
)

func main() {
	cli := cmd.NewCLI()

	if err := cli.Execute(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
