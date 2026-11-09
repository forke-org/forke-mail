package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (v *verifier) tests(log, pkg string, required ...string) error {
	pattern := "^(" + strings.Join(required, "|") + ")$"
	if err := v.command(log, "go", "test", "-json", "-count=1", "-run", pattern, pkg); err != nil {
		return err
	}
	return requireTests(filepath.Join(v.dir, log+".txt"), required)
}

func requireTests(path string, required []string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	passed := map[string]bool{}
	for {
		var row struct{ Action, Test string }
		err := decoder.Decode(&row)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if row.Action == "pass" && row.Test != "" {
			passed[row.Test] = true
		}
	}
	for _, name := range required {
		if !passed[name] {
			return fmt.Errorf("required test %s did not run and pass", name)
		}
	}
	return nil
}
