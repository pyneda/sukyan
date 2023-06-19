package main

import (
	"github.com/pyneda/sukyan/cmd"
	"github.com/pyneda/sukyan/internal/config"
)

func main() {
	config.LoadConfig()
	cmd.Execute()
}
