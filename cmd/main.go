package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
)

const DirSeparator = string(os.PathSeparator)

const (
	red   = "\033[31m"
	green = "\033[32m"
	reset = "\033[0m"
)

var (
	dir        = flag.String("dir", ".", "Directory in which the search will be performed")
	file       = flag.String("file", "", "Target file name for search")
	extension  = flag.String("extensions", "", "Target file extensions separated by comma")
	extensions []string
)

type Config struct {
	dir        string
	file       string
	extensions []string
}

func parseCfg() (*Config, error) {
	flag.Parse()

	if *file == "" {
		return nil, fmt.Errorf("file cannot be empty or omitted")
	}

	if *extension != "" {
		for _, v := range strings.Split(*extension, ",") {
			if ext := strings.TrimSpace(v); ext != "" {
				extensions = append(extensions, ext)
			}
		}
	}

	return &Config{
		dir:        *dir,
		file:       *file,
		extensions: extensions,
	}, nil
}

func main() {
	cfg, err := parseCfg()
	if err != nil {
		log.Fatalln(err)
	}

	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, os.Interrupt, os.Kill)
	defer close(sigsCh)

	filesCh := make(chan string, 1)
	defer close(filesCh)

	errsCh := make(chan error, 1)
	defer close(errsCh)

	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go find(ctx, wg, filesCh, errsCh, cfg.file, cfg.dir)

	wg.Add(1)
	go func() {
		defer wg.Done()

		i := 1
		for {
			select {
			case <-sigsCh:
				log.Println("OS signal had been caught, interrupting...")
				return
			case <-ctx.Done():
				return
			case f := <-filesCh:
				log.Printf("%v#%d: %v%v\n", green, i, f, reset)
				i++
			case e := <-errsCh:
				log.Printf("%v%v%v", red, e, reset)
			}
		}
	}()

	wg.Wait()
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
				if dir == DirSeparator {
					intoDir += dirEntry.Name()
				} else {
					intoDir = dir + DirSeparator + dirEntry.Name()
				}

				wg.Add(1)
				go find(ctx, wg, filesCh, errsCh, file, intoDir)
			} else {
				if dirEntry.Name() == file {
					filesCh <- dir + DirSeparator + dirEntry.Name()
				}
			}
		}
	}
}
