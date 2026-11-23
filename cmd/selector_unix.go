//go:build !windows

package cmd

import (
	"os"

	"golang.org/x/term"
)

func prepareInteractiveTerminal(inFile, outFile *os.File) (restore func(), width, height int, err error) {
	inFd := int(inFile.Fd())
	state, err := term.MakeRaw(inFd)
	if err != nil {
		return nil, 0, 0, err
	}

	width, height = 120, 24
	if outFile != nil {
		if w, h, szErr := term.GetSize(int(outFile.Fd())); szErr == nil && w > 0 && h > 0 {
			width, height = w, h
		}
	}

	restore = func() {
		_ = term.Restore(inFd, state)
	}

	return restore, width, height, nil
}
