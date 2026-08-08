package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/bestony/imap-forwarder/internal/config"
	"github.com/bestony/imap-forwarder/internal/fetcher"
	"github.com/bestony/imap-forwarder/internal/writer"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to config.toml")
	verbose := flag.Bool("verbose", false, "enable debug logging")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if err := run(*configPath); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	slog.Info("config loaded",
		"path", configPath,
		"accounts", len(cfg.Accounts),
		"output_file", cfg.OutputFile,
		"max_body_bytes", cfg.MaxBodyBytes,
	)

	out, err := writer.Open(cfg.OutputFile)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			slog.Error("close output file", "err", closeErr)
		}
	}()

	var failed int
	var totalWritten int
	for _, account := range cfg.Accounts {
		slog.Info("processing account", "account", account.Name, "host", account.Host, "mailbox", account.Mailbox)
		n, err := fetcher.FetchAll(account, cfg.MaxBodyBytes, out.Append)
		totalWritten += n
		if err != nil {
			slog.Error("account failed", "account", account.Name, "err", err, "written", n)
			failed++
			continue
		}
		slog.Info("account done", "account", account.Name, "written", n)
	}

	slog.Info("all accounts processed", "total_written", totalWritten, "failed_accounts", failed)
	if failed > 0 {
		return fmt.Errorf("%d account(s) failed", failed)
	}
	return nil
}
