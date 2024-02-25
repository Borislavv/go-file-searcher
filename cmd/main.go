package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
)

const dirSeparator = string(os.PathSeparator)

const (
	red   = "\033[31m"
	green = "\033[32m"
	reset = "\033[0m"
)

var (
	dir    = flag.String("dir", ".", "Directory in which the search will be performed")
	file   = flag.String("file", "", "Target file name for search")
	isErrs = flag.Bool("errs", true, "Determines whether errors should be displayed")
)

func main() {
	flag.Parse()
	if *file == "" {
		log.Fatalln("file cannot be empty or omitted")
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
			log.Printf("%v#%d: %v%v\n", green, i, f, reset)
			i++
		}
	}()

	go func() {
		for e := range errsCh {
			if *isErrs {
				log.Printf("%v%v%v", red, e, reset)
			}
		}
	}()

	go func() {
		wg.Wait()
		sigsCh <- os.Interrupt
	}()

	<-sigsCh
	cancel()
}

func find(
	ctx context.Context,
	wg *sync.WaitGroup,
	filesCh chan<- string,
	errsCh chan<- error,
	file string,
	dir string,
) {
	defer wg.Done()

	select {
	case <-ctx.Done():
		return
	default:
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			errsCh <- err
			return
		}

		for _, dirEntry := range dirEntries {
			if dirEntry.IsDir() {
				var intoDir = dir
				if dir == dirSeparator {
					intoDir += dirEntry.Name()
				} else {
					intoDir = dir + dirSeparator + dirEntry.Name()
				}

				wg.Add(1)
				go find(ctx, wg, filesCh, errsCh, file, intoDir)
			} else {
				if dirEntry.Name() == file {
					filesCh <- dir + dirSeparator + dirEntry.Name()
				}
			}
		}
	}
}
