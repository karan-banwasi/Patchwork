package cmd

import (
	"fmt"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

const (
	DefaultMinNameWidth      = 10
	DefaultMinIDWidth        = 10
	DefaultMinVersionWidth   = 7
	DefaultMinAvailableWidth = 9
)

// getDefaultPackageColumnWidths calculates padding widths using default minimum column boundaries.
func getDefaultPackageColumnWidths(updates []pm.PackageUpdate) (int, int, int, int) {
	return getPackageColumnWidths(updates, DefaultMinNameWidth, DefaultMinIDWidth, DefaultMinVersionWidth, DefaultMinAvailableWidth)
}

// getPackageColumnWidths calculates the required padding widths for tabular alignment of packages.
func getPackageColumnWidths(updates []pm.PackageUpdate, minName, minId, minVer, minAvail int) (int, int, int, int) {
	var maxNameLen, maxIdLen, maxVersionLen, maxAvailLen int
	for _, pkg := range updates {
		if len(pkg.Name) > maxNameLen {
			maxNameLen = len(pkg.Name)
		}
		if len(pkg.Id) > maxIdLen {
			maxIdLen = len(pkg.Id)
		}
		if len(pkg.Version) > maxVersionLen {
			maxVersionLen = len(pkg.Version)
		}
		if len(pkg.AvailableVersion) > maxAvailLen {
			maxAvailLen = len(pkg.AvailableVersion)
		}
	}

	if maxNameLen < minName {
		maxNameLen = minName
	}
	if maxIdLen < minId {
		maxIdLen = minId
	}
	if maxVersionLen < minVer {
		maxVersionLen = minVer
	}
	if maxAvailLen < minAvail {
		maxAvailLen = minAvail
	}

	return maxNameLen, maxIdLen, maxVersionLen, maxAvailLen
}

// formatUpdateRow formats a single package update into a perfectly aligned tabular row string.
func formatUpdateRow(pkg pm.PackageUpdate, nameLen, idLen, verLen, availLen int) string {
	manager := pkg.ManagerName
	if manager == "" {
		manager = pkg.Source
	}
	if manager == "" {
		manager = "unknown"
	}
	formatStr := fmt.Sprintf("%%-%ds   %%-%ds   %%-%ds    %%-%ds   %%s", nameLen, idLen, verLen, availLen)
	return fmt.Sprintf(formatStr, pkg.Name, pkg.Id, pkg.Version, pkg.AvailableVersion, manager)
}
