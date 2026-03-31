package main

import (
	"fmt"
	"os"

	"github.com/kashportsa/kp-gruuk/internal/client"
)

func main() {
	if err := client.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
