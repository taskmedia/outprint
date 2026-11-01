// Command seed provisions the development stack: the fictional user, a bound
// and an unbound Collection, each with a Document. `task dev` and `task e2e:run`
// both run it. See docs/development.md.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/taskmedia/outprint/internal/devseed"
)

func main() { os.Exit(run()) }

func run() int {
	var (
		url      = flag.String("url", "http://localhost:8090", "origin of the injector")
		email    = flag.String("email", devseed.DefaultEmail, "dev user")
		password = flag.String("password", devseed.DefaultPassword, "dev password")
		content  = flag.String("content", "deploy/dev/seed", "directory with chart.png and diagram.svg")
		out      = flag.String("out", "deploy/dev/.generated/seed.json", "where to write the ids and URLs")
		wait     = flag.Duration("wait", 5*time.Minute, "how long to wait for the stack")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *wait)
	defer cancel()

	if err := devseed.WaitReady(ctx, *url, time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		return 1
	}
	res, err := devseed.Run(ctx, devseed.Config{
		BaseURL: *url, Email: *email, Password: *password, Content: os.DirFS(*content),
		Logf: func(f string, a ...any) { fmt.Fprintf(os.Stderr, "seed: "+f+"\n", a...) },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		return 1
	}
	if err := write(*out, res); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		return 1
	}
	fmt.Printf("\nOutline (dev):  %s\n", res.BaseURL)
	fmt.Printf("Sign in with:   %s / %s   (DEV ONLY, fictional)\n", res.Email, res.Password)
	fmt.Printf("Try:            %s%s  (has the PDF button)\n", res.BaseURL, res.Bound.DocumentPath)
	fmt.Printf("No button:      %s%s\n", res.BaseURL, res.Unbound.DocumentPath)
	return 0
}

func write(path string, res devseed.Result) error {
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
