// Command evaluation-live-cache runs the fixed, explicitly approved cache experiment.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/evaluation/livecache"
	"github.com/Tencent/WeKnora/internal/logger"
)

func main() {
	var options livecache.Options
	flag.BoolVar(&options.Execute, "execute", false,
		"perform the reviewed fixed plan; default is zero-network dry-run")
	flag.StringVar(&options.ConfirmPlanSHA256, "confirm-plan-sha256", "",
		"exact SHA-256 from the approved dry-run plan")
	flag.StringVar(&options.ApprovedCNY, "approve-cny", "", "explicit approved CNY cap, at most 3.00")
	flag.StringVar(&options.OutputDir, "out", "x08-artifacts", "new output directory; existing paths are rejected")
	keyEnv := flag.String("key-env", "X08_DASHSCOPE_API_KEY",
		"credential environment variable, read only after execute confirmation")
	keyStdin := flag.Bool("key-stdin", false, "read the credential from standard input after execute confirmation")
	flag.Parse()
	logger.SetLogLevel(logger.LevelError)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	result, err := livecache.Run(ctx, options, func() (string, error) {
		if *keyStdin {
			b, e := io.ReadAll(io.LimitReader(os.Stdin, 4097))
			if e != nil || len(b) > 4096 {
				return "", errors.New("invalid credential input")
			}
			return strings.TrimSpace(string(b)), nil
		}
		return os.Getenv(*keyEnv), nil
	})
	if result != nil {
		_ = json.NewEncoder(os.Stdout).Encode(result)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
