//go:build windows

package cmd

import (
	"github.com/karan-banwasi/patchwork/internal/cargo"
	"github.com/karan-banwasi/patchwork/internal/choco"
	"github.com/karan-banwasi/patchwork/internal/dotnet"
	"github.com/karan-banwasi/patchwork/internal/npm"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/psresource"
	"github.com/karan-banwasi/patchwork/internal/scoop"
	"github.com/karan-banwasi/patchwork/internal/uv"
	"github.com/karan-banwasi/patchwork/internal/winget"
)

func defaultManagers() []pm.Manager {
	return []pm.Manager{
		winget.NewWingetManager(),
		scoop.NewScoopManager(),
		choco.NewChocoManager(),
		psresource.NewPSResourceManager(),
		npm.NewNpmManager(),
		dotnet.NewDotnetManager(),
		uv.NewUvManager(),
		cargo.NewCargoManager(),
	}
}
