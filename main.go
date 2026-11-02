package main

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64,arm64,386

import "github.com/karan-banwasi/patchwork/cmd"

func main() {
	cmd.Execute()
}
