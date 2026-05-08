package main

//go:generate go run github.com/tc-hib/go-winres make

import "github.com/karan-banwasi/patchwork/cmd"

func main() {
	cmd.Execute()
}
