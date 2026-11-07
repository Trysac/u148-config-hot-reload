//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os/exec"
)

func main() {
	cmd := exec.Command("go", "test", "-v", ".")
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
