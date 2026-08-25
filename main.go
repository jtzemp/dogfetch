package main

import (
	"os"

	"github.com/jtzemp/dogfetch/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
