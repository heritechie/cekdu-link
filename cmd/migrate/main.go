package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/heritechie/cekdulu-link/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	dir := flag.String("dir", "migrations", "path to SQL migrations directory")
	flag.Parse()

	cmd := flag.Arg(0)
	if cmd == "" {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down|status>")
		os.Exit(1)
	}

	cfg := config.Load()

	db, err := sql.Open("pgx", cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}

	goose.SetDialect("postgres")

	switch cmd {
	case "up":
		err = goose.UpContext(ctx, db, *dir)
	case "down":
		err = goose.DownContext(ctx, db, *dir)
	case "status":
		err = goose.StatusContext(ctx, db, *dir)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\nusage: migrate <up|down|status>\n", cmd)
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("migration %s failed: %v", cmd, err)
	}

	log.Printf("migration %s completed", cmd)
}
