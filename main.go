package main

//go:generate go run github.com/tc-hib/go-winres make --arch amd64,arm64,386

import "github.com/karan-banwasi/patchwork/cmd"

func main() {
	cmd.Execute()
}
