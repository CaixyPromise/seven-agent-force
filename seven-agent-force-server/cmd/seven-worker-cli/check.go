package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"time"

	checkdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	checkruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/runtime"
)

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	input := fs.String("input", "", "frozen check input")
	repository := fs.String("repository", "/workspace", "private writable check workspace")
	source := fs.String("source", "", "read-only source to copy into private workspace")
	dependencies := fs.String("dependencies", "", "optional read-only prepared dependency directory")
	timeout := fs.Duration("timeout", 5*time.Minute, "check time limit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" || fs.NArg() != 0 {
		return errors.New("CHECK_ARGUMENT_INVALID")
	}
	f, err := os.Open(*input)
	if err != nil {
		return errors.New("CHECK_INPUT_UNAVAILABLE")
	}
	defer f.Close()
	var value checkdomain.Input
	decoder := json.NewDecoder(io.LimitReader(f, 128<<10))
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("CHECK_INPUT_INVALID")
	}
	if *source != "" {
		if err := checkruntime.Snapshot(context.Background(), *source, *repository); err != nil {
			return errors.New("CHECK_SNAPSHOT_UNAVAILABLE")
		}
	}
	if *dependencies != "" {
		if !filepath.IsAbs(*dependencies) || filepath.Clean(*dependencies) != *dependencies {
			return errors.New("CHECK_DEPENDENCIES_INVALID")
		}
		info, statErr := os.Lstat(*dependencies)
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("CHECK_DEPENDENCIES_UNAVAILABLE")
		}
		if err := os.Symlink(*dependencies, filepath.Join(*repository, "node_modules")); err != nil {
			return errors.New("CHECK_DEPENDENCIES_UNAVAILABLE")
		}
	}
	return json.NewEncoder(os.Stdout).Encode(checkruntime.Shell(context.Background(), value, *repository, *timeout))
}
