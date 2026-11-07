//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os/exec"
)

func main() {
	cmd := exec.Command("go", "build", "./...")
	out, err := cmd.CombinedOutput()
	fmt.Printf("%s\n", out)
	if err != nil {
		fmt.Printf("Exit: %v\n", err)
	}
}
