package main

import "fmt"

// Version is the build-time version string. Override with -ldflags
// "-X main.Version=v1.2.3" at release time.
var Version = "dev"

func cmdVersion(_ []string) int {
	fmt.Printf("hostmux %s\n", Version)
	return 0
}
