package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"sync"
	"time"
)

const (
	pathSeparator = string(os.PathSeparator)
	rootDirectory = pathSeparator
)

const (
	red   = "\033[31m"
	green = "\033[32m"
	reset = "\033[0m"
)

var (
	dir    = flag.String("dir", ".", "Directory in which the search will be performed")
	file   = flag.String("file", "", "Target file name for search")
	isErrs = flag.Bool("errs", true, "Determines whether errors should be displayed")
	cpu    = flag.Uint("cpu", 0, "Max number of CPU to use (if value is 0 (zero) then will used max)")
	rgx    = flag.Bool("rgx", false, "If true, then file must contains a valid regex.")
)

var (
	regExpr *regexp.Regexp = nil
)

func main() {
	s := time.Now()
	defer func() { fmt.Printf("Elapsed: %v", time.Since(s)) }()

	flag.Parse()

	if *file == "" {
		log.Println("error: file cannot be empty or omitted")
		return
	}
	if *rgx {
		regExpr, err := regexp.Compile(*file)
		log.Println(err)
		return
	}

	if *cpu != 0 {
		runtime.GOMAXPROCS(int(*cpu))
	}

	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, os.Interrupt, os.Kill)
	defer close(sigsCh)

	filesCh := make(chan string)
	defer close(filesCh)

	errsCh := make(chan error)
	defer close(errsCh)

	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go find(ctx, wg, filesCh, errsCh, *file, *dir)

	go func() {
		i := 1
		for f := range filesCh {
			_, _ = fmt.Fprintf(os.Stdout, "%v#%d: %v%v\n", green, i, f, reset)
			i++
		}
	}()

	if *isErrs {
		go func() {
			for e := range errsCh {
				_, _ = fmt.Fprintf(os.Stderr, "%verror: %v%v\n", red, e, reset)
			}
		}()
	}

	go func() {
		wg.Wait()
		sigsCh <- os.Interrupt
	}()

	<-sigsCh
	cancel()
}

func find(ctx context.Context, wg *sync.WaitGroup, fCh chan<- string, eCh chan<- error, file, dir string) {
	defer wg.Done()

	select {
	case <-ctx.Done():
		return
	default:
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			if *isErrs {
				eCh <- err
			}
			return
		}

		for _, dirEntry := range dirEntries {
			if regExpr != nil {
				if regExpr.MatchString(dirEntry.Name()) {
					fCh <- dir + pathSeparator + dirEntry.Name()
				}
			} else if dirEntry.Name() == file {
				fCh <- dir + pathSeparator + dirEntry.Name()
			}

			if info, err := dirEntry.Info(); err != nil {
				if *isErrs {
					eCh <- err
				}
				continue
			} else {
				if info.Size() == 0 {
					continue
				}
			}

			if dirEntry.IsDir() {
				wg.Add(1)
				if dir == rootDirectory {
					go find(ctx, wg, fCh, eCh, file, dir+dirEntry.Name())
				} else {
					go find(ctx, wg, fCh, eCh, file, dir+pathSeparator+dirEntry.Name())
				}
			}
		}
	}
}
