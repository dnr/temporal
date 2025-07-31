package main

import (
	"os"

	"go.temporal.io/server/tools/fairsim"
)

func main() {
	if err := fairsim.RunTool(os.Args); err != nil {
		os.Exit(1)
	}
}
