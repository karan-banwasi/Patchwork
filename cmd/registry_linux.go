//go:build linux

package cmd

import (
	"github.com/karan-banwasi/patchwork/internal/apt"
	"github.com/karan-banwasi/patchwork/internal/brew"
	"github.com/karan-banwasi/patchwork/internal/dnf"
	"github.com/karan-banwasi/patchwork/internal/flatpak"
	"github.com/karan-banwasi/patchwork/internal/pacman"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/snap"
	"github.com/karan-banwasi/patchwork/internal/zypper"
)

func defaultManagers() []pm.Manager {
	linuxManagers := []pm.Manager{
		brew.NewBrewManager(),
		apt.NewAptManager(),
		dnf.NewDnfManager(),
		pacman.NewPacmanManager(),
		zypper.NewZypperManager(),
		flatpak.NewFlatpakManager(),
		snap.NewSnapManager(),
	}
	return append(linuxManagers, crossPlatformManagers()...)
}
