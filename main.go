package main

import "github.com/zwindler/kubectl-debug-pvc/cmd"

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	cmd.Execute(version)
}
