package main

import (
	"github.com/pyneda/sukyan/cmd"
	"github.com/pyneda/sukyan/lib/config"
)

func main() {
	config.LoadConfig()
	cmd.Execute()
}
