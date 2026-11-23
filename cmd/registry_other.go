//go:build !windows && !darwin && !linux

package cmd

import (
	"github.com/karan-banwasi/patchwork/internal/brew"
	"github.com/karan-banwasi/patchwork/internal/pm"
)

func defaultManagers() []pm.Manager {
	return append([]pm.Manager{brew.NewBrewManager()}, crossPlatformManagers()...)
}
