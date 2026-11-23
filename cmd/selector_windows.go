//go:build windows

package cmd

import (
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

func prepareInteractiveTerminal(inFile, outFile *os.File) (restore func(), width, height int, err error) {
	inFd := int(inFile.Fd())
	state, err := term.MakeRaw(inFd)
	if err != nil {
		return nil, 0, 0, err
	}

	inHandle := windows.Handle(inFile.Fd())
	var origInMode uint32
	hasOrigInMode := windows.GetConsoleMode(inHandle, &origInMode) == nil
	if hasOrigInMode {
		_ = windows.SetConsoleMode(inHandle, origInMode|windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	}

	var origOutMode uint32
	var outHandle windows.Handle
	hasOrigOutMode := false
	if outFile != nil {
		outHandle = windows.Handle(outFile.Fd())
		if windows.GetConsoleMode(outHandle, &origOutMode) == nil {
			hasOrigOutMode = true
			_ = windows.SetConsoleMode(outHandle, origOutMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.ENABLE_PROCESSED_OUTPUT)
		}
	}

	width, height = 120, 24
	if outFile != nil {
		if w, h, szErr := term.GetSize(int(outFile.Fd())); szErr == nil && w > 0 && h > 0 {
			width, height = w, h
		}
	}

	restore = func() {
		if hasOrigOutMode {
			_ = windows.SetConsoleMode(outHandle, origOutMode)
		}
		if hasOrigInMode {
			_ = windows.SetConsoleMode(inHandle, origInMode)
		}
		_ = term.Restore(inFd, state)
	}

	return restore, width, height, nil
}
