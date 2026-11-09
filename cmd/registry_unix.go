//go:build !windows

package cmd

import (
	"github.com/karan-banwasi/patchwork/internal/brew"
	"github.com/karan-banwasi/patchwork/internal/cargo"
	"github.com/karan-banwasi/patchwork/internal/dotnet"
	"github.com/karan-banwasi/patchwork/internal/npm"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/uv"
)

func defaultManagers() []pm.Manager {
	return []pm.Manager{
		brew.NewBrewManager(),
		npm.NewNpmManager(),
		dotnet.NewDotnetManager(),
		uv.NewUvManager(),
		cargo.NewCargoManager(),
	}
}
