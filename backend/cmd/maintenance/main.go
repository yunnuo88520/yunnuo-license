package main

import (
	"context"
	"encoding/json"
	"filippo.io/age"
	"flag"
	"fmt"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/config"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/maintenance"
	"log"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: yunnuo-ops keygen|backup|restore")
	}
	command := os.Args[1]
	f := flag.NewFlagSet(command, flag.ExitOnError)
	output := f.String("output", "", "new backup/key file")
	recipient := f.String("recipient", "", "age public recipient")
	input := f.String("input", "", "encrypted backup")
	identity := f.String("identity", "", "private age identity file")
	destination := f.String("destination", "", "new restoration directory")
	dsn := f.String("mysql-dsn-file", "", "0600 file containing an empty target MySQL database DSN")
	table := f.String("table", "operation_logs", "log table: operation_logs, audit_logs, email_logs")
	keepDays := f.Int("keep-days", 365, "days of online log retention (minimum 30)")
	batch := f.Int("batch", 1000, "maximum log records in one archive")
	max := f.Int64("max-bytes", 64<<30, "maximum uncompressed restore size")
	f.Parse(os.Args[2:])
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	switch command {
	case "keygen":
		if *output == "" {
			log.Fatal("--output required")
		}
		key, err := age.GenerateX25519Identity()
		if err != nil {
			log.Fatal(err)
		}
		file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatal(err)
		}
		_, err = file.WriteString(key.String() + "\n")
		if err != nil {
			log.Fatal(err)
		}
		if err = file.Close(); err != nil {
			log.Fatal(err)
		}
		fmt.Println(key.Recipient().String())
	case "archive-logs":
		if *output == "" || *recipient == "" {
			log.Fatal("--output and --recipient required")
		}
		cfg, err := config.Load()
		if err != nil {
			log.Fatal(err)
		}
		n, err := maintenance.ArchiveLogs(ctx, cfg, *table, *recipient, *output, *keepDays, *batch)
		if err != nil {
			log.Fatal(err)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"result": "archived", "records": n})
	case "backup":
		if *output == "" || *recipient == "" {
			log.Fatal("--output and --recipient required")
		}
		cfg, err := config.Load()
		if err != nil {
			log.Fatal(err)
		}
		m, err := maintenance.Backup(ctx, cfg, *recipient, *output)
		if err != nil {
			log.Fatal(err)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"result": "backed_up", "driver": m.Driver, "files": len(m.Files), "created_at": m.CreatedAt})
	case "restore":
		if *input == "" || *identity == "" || *destination == "" {
			log.Fatal("--input, --identity and --destination required")
		}
		m, err := maintenance.Restore(ctx, maintenance.RestoreOptions{Input: *input, IdentityFile: *identity, Destination: *destination, MySQLDSNFile: *dsn, MaxBytes: *max})
		if err != nil {
			log.Fatal(err)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"result": "restored", "driver": m.Driver, "files": len(m.Files), "created_at": m.CreatedAt})
	default:
		log.Fatal("unknown maintenance command")
	}
}
