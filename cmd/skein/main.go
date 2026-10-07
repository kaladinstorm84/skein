package main

import (
	"os"

	"skein/internal/abi"
)

func main() {
	out, code := abi.Run(os.Args[1:], os.Stdin)
	abi.Write(os.Stdout, out)
	os.Exit(code)
}
