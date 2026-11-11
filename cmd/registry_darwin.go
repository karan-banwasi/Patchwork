//go:build darwin

package cmd

import (
	"github.com/karan-banwasi/patchwork/internal/brew"
	"github.com/karan-banwasi/patchwork/internal/mas"
	"github.com/karan-banwasi/patchwork/internal/pm"
)

func defaultManagers() []pm.Manager {
	darwinManagers := []pm.Manager{
		brew.NewBrewManager(),
		mas.NewMasManager(),
	}
	return append(darwinManagers, crossPlatformManagers()...)
}
