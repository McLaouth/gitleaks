package detect

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/report"
)

// FromFiles opens the directory or file specified in source and checks each file against the rules
// from the configuration. If any secrets are found, they are added to the list of findings.
func FromFiles(source string, cfg config.Config, outputOptions Options) ([]report.Finding, error) {
	var (
		findings []report.Finding
		mu       sync.Mutex
	)
	concurrentGoroutines := make(chan struct{}, MAXGOROUTINES)
	g, _ := errgroup.WithContext(context.Background())
	paths := make(chan string)
	g.Go(func() error {
		defer close(paths)
		return filepath.Walk(source,
			func(path string, fInfo os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if fInfo.Name() == ".git" {
					return filepath.SkipDir
				}
				if fInfo.Mode().IsRegular() {
					paths <- path
				}
				return nil
			})
	})
	for pa := range paths {
		p := pa
		concurrentGoroutines <- struct{}{}
		g.Go(func() error {
			defer func() {
				<-concurrentGoroutines
			}()
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			fis := DetectFindings(cfg, b, p, "")
			for _, fi := range fis {
				// need to add 1 since line counting starts at 1
				fi.StartLine++
				fi.EndLine++

				if outputOptions.Redact {
					fi.Redact()
				}
				if outputOptions.Verbose {
					printFinding(fi)
				}
				mu.Lock()
				findings = append(findings, fi)
				mu.Unlock()
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return findings, err
	}

	return findings, nil
}
