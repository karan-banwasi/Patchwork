package cmd

import (
	"github.com/karan-banwasi/patchwork/internal/asdf"
	"github.com/karan-banwasi/patchwork/internal/bun"
	"github.com/karan-banwasi/patchwork/internal/cargo"
	"github.com/karan-banwasi/patchwork/internal/dotnet"
	"github.com/karan-banwasi/patchwork/internal/gem"
	"github.com/karan-banwasi/patchwork/internal/mise"
	"github.com/karan-banwasi/patchwork/internal/npm"
	"github.com/karan-banwasi/patchwork/internal/pipx"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/pnpm"
	"github.com/karan-banwasi/patchwork/internal/rustup"
	"github.com/karan-banwasi/patchwork/internal/uv"
	"github.com/karan-banwasi/patchwork/internal/vscode"
)

func crossPlatformManagers() []pm.Manager {
	return []pm.Manager{
		npm.NewNpmManager(),
		pnpm.NewPnpmManager(),
		bun.NewBunManager(),
		dotnet.NewDotnetManager(),
		uv.NewUvManager(),
		pipx.NewPipxManager(),
		cargo.NewCargoManager(),
		rustup.NewRustupManager(),
		gem.NewGemManager(),
		mise.NewMiseManager(),
		asdf.NewAsdfManager(),
		vscode.NewVSCodeManager(),
	}
}
