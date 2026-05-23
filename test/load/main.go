package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/maansaake/arbiter"
	"github.com/maansaake/arbiter/pkg/module"
)

const (
	defaultErrorLogPath = "build/load-test/error.log"
	defaultInfoLogPath  = "build/load-test/info.log"
)

func main() {
	err := arbiter.Run(module.Modules{newLocksmithLoadModule()}, &arbiter.Opts{
		ErrorLogPath: defaultErrorLogPath,
		InfoLogPath:  defaultInfoLogPath,
	})
	if err == nil {
		os.Exit(0)
	}

	if errors.Is(err, arbiter.ErrStopping) {
		fmt.Fprintf(os.Stderr, "Arbiter stopped with error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Error running Arbiter: %v\n", err)
	os.Exit(1)
}
