package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/yashikota/shippo/internal/daemon"
)

var Version string

func getVersion() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "(devel)" {
			return info.Main.Version
		}
		if v, ok := getVCSBuildVersion(info); ok {
			return v
		}
	}
	return "(unset)"
}

func getVCSBuildVersion(info *debug.BuildInfo) (string, bool) {
	var (
		revision string
		dirty    string
	)
	for _, v := range info.Settings {
		switch v.Key {
		case "vcs.revision":
			revision = v.Value
		case "vcs.modified":
			if v.Value == "true" {
				dirty = " (dirty)"
			}
		}
	}
	if revision == "" {
		return "", false
	}
	return revision + dirty, true
}

func main() {
	app := &cli.Command{
		Name:    "shippo",
		Usage:   "Auto serve localhost ports via Tailscale using eBPF",
		Version: getVersion(),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "verbose debug logging",
			},
			&cli.StringFlag{
				Name:    "log-file",
				Aliases: []string{"l"},
				Usage:   "log file path",
				Value:   "/tmp/shippo.log",
				Sources: cli.EnvVars("SHIPPO_LOG_FILE"),
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "config file path",
				Sources: cli.EnvVars("SHIPPO_CONFIG"),
			},
		},
		Commands: []*cli.Command{
			{
				Name:   "daemon",
				Usage:  "Run as daemon (default when no subcommand)",
				Action: daemonAction,
			},
			{
				Name:   "status",
				Usage:  "Show currently served ports",
				Action: statusAction,
			},
			{
				Name:      "add",
				Usage:     "Add port spec(s) to the allow list (e.g. 3000, 8000-8999, *)",
				ArgsUsage: "<spec> [spec...]",
				Action:    addAction,
			},
			{
				Name:      "remove",
				Usage:     "Remove port spec(s) from the allow list",
				ArgsUsage: "<spec> [spec...]",
				Action:    removeAction,
			},
			{
				Name:   "once",
				Usage:  "Sync once and exit (no daemon)",
				Action: onceAction,
			},
			{
				Name:   "rules",
				Usage:  "Show current port allow rules",
				Action: rulesAction,
			},
			{
				Name:   "config-path",
				Usage:  "Print the config file path",
				Action: configPathAction,
			},
			{
				Name:   "urls",
				Usage:  "Print all currently served URLs",
				Action: urlsAction,
			},
			{
				Name:   "last",
				Usage:  "Print the most recently served URL",
				Action: lastAction,
			},
		},
		DefaultCommand: "daemon",
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func setupLogging(logPath string) {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[shippo] ")

	if logPath == "" {
		log.SetOutput(os.Stderr)
		return
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Printf("warning: cannot open log file %s: %v", logPath, err)
		return
	}

	mw := io.MultiWriter(os.Stderr, logFile)
	log.SetOutput(mw)
}

func applyGlobalFlags(cmd *cli.Command) {
	daemon.Verbose = cmd.Bool("verbose")
	if c := cmd.String("config"); c != "" {
		daemon.ConfigPath = c
	}
}

func daemonAction(ctx context.Context, cmd *cli.Command) error {
	setupLogging(cmd.String("log-file"))
	applyGlobalFlags(cmd)

	if err := checkCapabilities(); err != nil {
		return err
	}

	return daemon.Run()
}

func statusAction(ctx context.Context, cmd *cli.Command) error {
	applyGlobalFlags(cmd)
	daemon.Status()
	return nil
}

func addAction(ctx context.Context, cmd *cli.Command) error {
	applyGlobalFlags(cmd)
	args := cmd.Args().Slice()
	if len(args) == 0 {
		return fmt.Errorf("usage: shippo add <spec> [spec...] (e.g. 3000, 8000-8999, *)")
	}
	return daemon.Add(args)
}

func removeAction(ctx context.Context, cmd *cli.Command) error {
	applyGlobalFlags(cmd)
	args := cmd.Args().Slice()
	if len(args) == 0 {
		return fmt.Errorf("usage: shippo remove <spec> [spec...]")
	}
	return daemon.Remove(args)
}

func rulesAction(ctx context.Context, cmd *cli.Command) error {
	applyGlobalFlags(cmd)
	daemon.Rules()
	return nil
}

func configPathAction(ctx context.Context, cmd *cli.Command) error {
	applyGlobalFlags(cmd)
	fmt.Println(daemon.ConfigFilePath())
	return nil
}

func urlsAction(ctx context.Context, cmd *cli.Command) error {
	urls, err := daemon.URLs()
	if err != nil {
		return err
	}
	for _, u := range urls {
		fmt.Println(u)
	}
	return nil
}

func lastAction(ctx context.Context, cmd *cli.Command) error {
	url, err := daemon.LastURL()
	if err != nil {
		return err
	}
	fmt.Println(url)
	return nil
}

func onceAction(ctx context.Context, cmd *cli.Command) error {
	setupLogging(cmd.String("log-file"))
	applyGlobalFlags(cmd)

	if err := checkCapabilities(); err != nil {
		return err
	}

	daemon.RunOnce()
	fmt.Println("Sync complete.")
	return nil
}

func checkCapabilities() error {
	if os.Geteuid() == 0 {
		return nil
	}

	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return fmt.Errorf("cannot read /proc/self/status: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] != "0000000000000000" {
				return nil
			}
		}
	}

	return fmt.Errorf("insufficient privileges: run as root or with CAP_BPF+CAP_NET_ADMIN+CAP_PERFMON")
}
