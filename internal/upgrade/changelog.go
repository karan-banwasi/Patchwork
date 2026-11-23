package upgrade

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

// ChangelogURL returns a deterministic ecosystem-specific release or package URL
// for pkg without performing any network calls. Returns "" when no mapping exists.
func ChangelogURL(pkg pm.PackageUpdate) string {
	id := strings.TrimSpace(pkg.Id)
	if id == "" {
		id = strings.TrimSpace(pkg.Name)
	}
	if id == "" {
		return ""
	}

	mgr := strings.ToLower(strings.TrimSpace(pkg.ManagerName))
	switch mgr {
	case "npm", "pnpm", "pnpm-global", "bun", "bun-global":
		return "https://www.npmjs.com/package/" + escapePathPreserveSlash(id)
	case "cargo", "rust":
		return "https://crates.io/crates/" + url.PathEscape(id)
	case "uv", "uv-tool", "pipx", "pipx-app", "pypi", "python":
		return "https://pypi.org/project/" + url.PathEscape(id) + "/"
	case "gem", "rubygems", "ruby":
		return "https://rubygems.org/gems/" + url.PathEscape(id)
	case "dotnet", "dotnet-tool":
		return "https://www.nuget.org/packages/" + url.PathEscape(id)
	case "brew", "homebrew":
		if strings.EqualFold(strings.TrimSpace(pkg.Source), "cask") {
			return "https://formulae.brew.sh/cask/" + url.PathEscape(id)
		}
		return "https://formulae.brew.sh/formula/" + url.PathEscape(id)
	case "scoop":
		return "https://scoop.sh/#/apps?q=" + url.QueryEscape(id)
	case "winget":
		if parts := strings.SplitN(id, ".", 2); len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return "https://winget.run/pkg/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1])
		}
		return "https://winget.run/search?query=" + url.QueryEscape(id)
	case "choco", "chocolatey":
		return "https://community.chocolatey.org/packages/" + url.PathEscape(id)
	case "psresource", "powershell", "psresourceget":
		return "https://www.powershellgallery.com/packages/" + url.PathEscape(id)
	case "vscode", "code", "vs-code":
		return "https://marketplace.visualstudio.com/items?itemName=" + url.QueryEscape(id)
	case "flatpak", "flathub":
		return "https://flathub.org/apps/" + url.PathEscape(id)
	case "snap", "snapcraft", "snappy":
		return "https://snapcraft.io/" + url.PathEscape(id)
	case "apt", "apt-get", "deb", "debian", "ubuntu":
		return "https://tracker.debian.org/pkg/" + url.PathEscape(id)
	case "dnf", "yum", "fedora", "rhel":
		return "https://packages.fedoraproject.org/pkgs/" + url.PathEscape(id) + "/"
	case "pacman", "arch":
		return "https://archlinux.org/packages/?q=" + url.QueryEscape(id)
	case "zypper", "opensuse", "suse":
		return "https://software.opensuse.org/package/" + url.PathEscape(id)
	case "mas", "macappstore", "appstore":
		cleanID := strings.TrimPrefix(id, "id")
		return "https://apps.apple.com/app/id" + url.PathEscape(cleanID)
	case "rustup", "rust-toolchain":
		return "https://releases.rs/"
	case "mise", "rtx":
		return "https://mise.jdx.dev/"
	case "asdf":
		return "https://asdf-vm.com/"
	default:
		return ""
	}
}

func escapePathPreserveSlash(s string) string {
	parts := strings.Split(s, "/")
	for i, part := range parts {
		// Preserve leading '@' for scoped npm packages (e.g. @types/node)
		if i == 0 && strings.HasPrefix(part, "@") {
			parts[i] = "@" + url.PathEscape(part[1:])
		} else {
			parts[i] = url.PathEscape(part)
		}
	}
	return strings.Join(parts, "/")
}

// RenderChangelogPreview writes a formatted table of bump levels and changelog/release URLs to out.
func RenderChangelogPreview(out io.Writer, pkgs []pm.PackageUpdate) {
	if out == nil || len(pkgs) == 0 {
		return
	}

	type row struct {
		id      string
		mgr     string
		bump    string
		verDiff string
		url     string
	}

	rows := make([]row, 0, len(pkgs))
	maxIdLen := 10
	maxMgrLen := 7
	maxBumpLen := 5
	maxVerLen := 7

	for _, pkg := range pkgs {
		bump := "-"
		verDiff := "-"
		if pkg.Version != "" || pkg.AvailableVersion != "" {
			bump = string(version.DiffLevel(pkg.ManagerName, pkg.Version, pkg.AvailableVersion))
			verDiff = fmt.Sprintf("%s -> %s", pkg.Version, pkg.AvailableVersion)
		}
		u := ChangelogURL(pkg)
		if u == "" {
			u = "-"
		}
		r := row{
			id:      pkg.Id,
			mgr:     pkg.ManagerName,
			bump:    bump,
			verDiff: verDiff,
			url:     u,
		}
		rows = append(rows, r)

		if n := utf8.RuneCountInString(r.id); n > maxIdLen {
			maxIdLen = n
		}
		if n := utf8.RuneCountInString(r.mgr); n > maxMgrLen {
			maxMgrLen = n
		}
		if n := utf8.RuneCountInString(r.bump); n > maxBumpLen {
			maxBumpLen = n
		}
		if n := utf8.RuneCountInString(r.verDiff); n > maxVerLen {
			maxVerLen = n
		}
	}

	fmt.Fprintln(out, "\n=== Changelog Preview ===")
	rowFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds   %%-%ds   %%s\n", maxIdLen, maxMgrLen, maxBumpLen, maxVerLen)
	fmt.Fprintf(out, rowFmt, "Package ID", "Manager", "Level", "Version", "Release / Package URL")
	fmt.Fprintf(out, rowFmt, strings.Repeat("-", maxIdLen), strings.Repeat("-", maxMgrLen), strings.Repeat("-", maxBumpLen), strings.Repeat("-", maxVerLen), "---------------------")
	for _, r := range rows {
		fmt.Fprintf(out, rowFmt, r.id, r.mgr, r.bump, r.verDiff, r.url)
	}
}
