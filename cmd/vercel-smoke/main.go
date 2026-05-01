package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sksareen/sauron/internal/daemon"
	"github.com/sksareen/sauron/internal/store"
)

// Standalone smoke test for the live:vercel source. Uses a temp SQLite file
// (not HOME override, so the vercel CLI still sees its auth dir).
const smokeSchema = `
CREATE TABLE IF NOT EXISTS live_vercel_logs (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    deployment_url TEXT NOT NULL,
    level          TEXT,
    method         TEXT,
    path           TEXT,
    status_code    INTEGER,
    message        TEXT NOT NULL,
    dedupe_hash    TEXT NOT NULL,
    captured_at    INTEGER NOT NULL,
    UNIQUE(dedupe_hash)
);
CREATE INDEX IF NOT EXISTS idx_vercel_captured_at ON live_vercel_logs(captured_at DESC);
`

func main() {
	url := os.Getenv("SAURON_VERCEL_DEPLOYMENT_URL")
	if url == "" {
		url = daemon.DefaultVercelDeploymentURL
	}
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	tmpDir, err := os.MkdirTemp("", "sauron-smoke-db-")
	if err != nil {
		log.Fatal(err)
	}
	dbPath := filepath.Join(tmpDir, "smoke.db")
	fmt.Println("smoke db:", dbPath)
	fmt.Println("watching:", url)

	sqldb, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	if _, err := sqldb.Exec(smokeSchema); err != nil {
		log.Fatalf("schema: %v", err)
	}
	db := &store.DB{DB: sqldb}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	if err := daemon.EnsureVercelCLI(ctx); err != nil {
		log.Fatalf("ensure cli: %v", err)
	}

	go daemon.RunVercelPoller(ctx, db, url)

	<-ctx.Done()

	rows, err := store.GetRecentVercelLogs(db, 20)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	fmt.Printf("\n=== %d rows captured ===\n", len(rows))
	for _, r := range rows {
		msg := r.Message
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		fmt.Printf("[%d] lvl=%s method=%s path=%s code=%d msg=%s\n",
			r.CapturedAt, r.Level, r.Method, r.Path, r.StatusCode, msg)
	}
}
