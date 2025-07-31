package main

import (
	"os"

	"go.temporal.io/server/tools/fairsim"
)

func main() {
	if err := fairsim.RunTool(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
