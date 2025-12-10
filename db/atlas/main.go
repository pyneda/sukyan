package main

import (
	"fmt"
	"io"
	"os"

	"ariga.io/atlas-provider-gorm/gormschema"
	"github.com/pyneda/sukyan/db"
)

func main() {
	stmts, err := gormschema.New("postgres").Load(
		&db.Workspace{},
		&db.User{},
		&db.RefreshToken{},
		&db.WorkerNode{},
		&db.Scan{},
		&db.ScanJob{},
		&db.PlaygroundCollection{},
		&db.PlaygroundSession{},
		&db.Task{},
		&db.TaskJob{},
		&db.History{},
		&db.WebSocketConnection{},
		&db.WebSocketMessage{},
		&db.JsonWebToken{},
		&db.WorkspaceCookie{},
		&db.StoredBrowserActions{},
		&db.Issue{},
		&db.OOBTest{},
		&db.OOBInteraction{},
		&db.BrowserEvent{},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load gorm schema: %v\n", err)
		os.Exit(1)
	}
	io.WriteString(os.Stdout, stmts)
}
