// Command leo is a personal, extensible task-runner CLI ("a helpful assistant").
package main

import "leo/internal/cli"

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cli.Execute(version)
}
