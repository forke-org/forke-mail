package main

import (
	"errors"
	"fmt"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func uniqueMigrations() error {
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return err
	}
	seen := map[int]string{}
	for _, f := range files {
		n, err := strconv.Atoi(strings.SplitN(filepath.Base(f), "_", 2)[0])
		if err != nil {
			return err
		}
		if prior := seen[n]; prior != "" {
			return fmt.Errorf("duplicate version %d: %s and %s", n, prior, f)
		}
		seen[n] = f
	}
	if len(seen) < 10 {
		return errors.New("expected at least 10 migrations")
	}
	return nil
}

func sourceChecks() error {
	routes := map[string]string{}
	banned := map[string]bool{"Development" + "Stub": true, "fixture" + "Engine": true}
	for _, dir := range []string{"internal", "cmd/server"} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var scan scanner.Scanner
			set := token.NewFileSet()
			scan.Init(set.AddFile(path, -1, len(data)), data, nil, 0)
			type lexeme struct {
				kind token.Token
				text string
			}
			var tokens []lexeme
			for {
				_, kind, text := scan.Scan()
				if kind == token.EOF {
					break
				}
				if kind == token.IDENT && banned[text] {
					return fmt.Errorf("runtime stub reference in %s", path)
				}
				tokens = append(tokens, lexeme{kind, text})
			}
			for i := 1; i+3 < len(tokens); i++ {
				if tokens[i-1].kind != token.PERIOD || tokens[i].kind != token.IDENT || tokens[i+1].kind != token.LPAREN {
					continue
				}
				method := tokens[i].text
				index := i + 2
				switch method {
				case "Get", "Post", "Put", "Patch", "Delete", "Head", "Options", "Handle":
				case "Method":
					if index+4 < len(tokens) && tokens[index].text == "http" && tokens[index+1].kind == token.PERIOD {
						method = strings.TrimPrefix(tokens[index+2].text, "Method")
						index += 4
					} else if index+2 < len(tokens) && tokens[index].kind == token.STRING {
						method, _ = strconv.Unquote(tokens[index].text)
						index += 2
					} else {
						return fmt.Errorf("unverifiable method in %s", path)
					}
				default:
					continue
				}
				if index >= len(tokens) || tokens[index].kind != token.STRING {
					continue
				}
				pattern, err := strconv.Unquote(tokens[index].text)
				if err != nil {
					return err
				}
				if !strings.HasPrefix(pattern, "/") {
					continue
				}
				key := strings.ToUpper(method) + " " + pattern
				if prior := routes[key]; prior != "" {
					return fmt.Errorf("duplicate route %s in %s and %s", key, prior, path)
				}
				routes[key] = path
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if len(routes) < 40 {
		return errors.New("route scan found unexpectedly few registrations")
	}
	return nil
}
