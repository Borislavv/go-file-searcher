package main

import (
	"context"
	"flag"
	"fmt"
	"golang.org/x/sys/unix"
	"log"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
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

// debug
var (
	active, deferred = atomic.Int64{}, atomic.Int64{}
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
		regex, err := regexp.Compile(*file)
		if err != nil {
			log.Println(err)
			return
		}
		regExpr = regex
	}

	if *cpu != 0 {
		runtime.GOMAXPROCS(int(*cpu))
	}

	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, os.Interrupt, os.Kill)
	defer close(sigsCh)

	filesCh := make(chan string, 1000)
	defer close(filesCh)

	errsCh := make(chan error, 1000)
	defer close(errsCh)

	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	active.Add(1)
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

	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-sigsCh:
				return
			case <-t.C:
				a := active.Load()
				d := deferred.Load()
				fmt.Printf("active: %d, deferred: %d, diff: %d\n", a, d, a-d)
			}
		}
	}()

	<-sigsCh
}

func find(ctx context.Context, wg *sync.WaitGroup, filesCh chan<- string, errsCh chan<- error, file, dir string) {
	defer func() {
		deferred.Add(1)
		wg.Done()
	}()

	select {
	case <-ctx.Done():
		return
	default:
		dirEntries, readDirErr := os.ReadDir(dir)
		if readDirErr != nil {
			if *isErrs {
				errsCh <- readDirErr
			}
			return
		}

		for _, dirEntry := range dirEntries {
			if isSymlinkOrIrregular(dirEntry.Type()) {
				continue
			}

			info, infoErr := dirEntry.Info()
			if infoErr != nil {
				if *isErrs {
					errsCh <- infoErr
				}
				continue
			} else {
				if info.Size() == 0 {
					continue
				}
			}

			if !isReadable(dir + "/" + info.Name()) {
				continue
			}

			if dirEntry.Type().IsRegular() {
				if regExpr != nil {
					if regExpr.MatchString(dirEntry.Name()) {
						filesCh <- dir + pathSeparator + dirEntry.Name()
					}
				} else if dirEntry.Name() == file {
					filesCh <- dir + pathSeparator + dirEntry.Name()
				}
			}

			if dirEntry.IsDir() {
				var intoDir string
				if dir == rootDirectory {
					intoDir = dir + dirEntry.Name()
				} else {
					intoDir = dir + pathSeparator + dirEntry.Name()
				}

				active.Add(1)
				wg.Add(1)
				go find(ctx, wg, filesCh, errsCh, file, intoDir)
			}
		}
	}
}

func isReadable(path string) bool {
	return unix.Access(path, unix.R_OK) == nil
}

func isSymlinkOrIrregular(mode os.FileMode) bool {
	if mode.Type()&os.ModeSymlink != 0 {
		return true
	}
	if mode.Type()&os.ModeIrregular != 0 {
		return true
	}
	return false
}
