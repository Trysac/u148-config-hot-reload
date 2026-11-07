//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	cmd := exec.Command("go", "run", "test_runner.go") // This is not right. I need to run test_runner.go which runs go test.
}
