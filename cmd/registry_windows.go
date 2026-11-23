//go:build windows

package cmd

import (
	"github.com/karan-banwasi/patchwork/internal/choco"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/psresource"
	"github.com/karan-banwasi/patchwork/internal/scoop"
	"github.com/karan-banwasi/patchwork/internal/winget"
)

func defaultManagers() []pm.Manager {
	windowsManagers := []pm.Manager{
		winget.NewWingetManager(),
		scoop.NewScoopManager(),
		choco.NewChocoManager(),
		psresource.NewPSResourceManager(),
	}
	return append(windowsManagers, crossPlatformManagers()...)
}
