package cmd

import (
	"fmt"
	"github.com/karan-banwasi/patchwork/internal/winget"
)

// getPackageColumnWidths calculates the required padding widths for tabular alignment of packages.
func getPackageColumnWidths(updates []winget.PackageUpdate, minName, minId, minVer int) (int, int, int) {
	var maxNameLen, maxIdLen, maxVersionLen int
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
	}

	if maxNameLen < minName { maxNameLen = minName }
	if maxIdLen < minId { maxIdLen = minId }
	if maxVersionLen < minVer { maxVersionLen = minVer }

	return maxNameLen, maxIdLen, maxVersionLen
}

// formatUpdateRow formats a single package update into a perfectly aligned tabular row string.
func formatUpdateRow(pkg winget.PackageUpdate, nameLen, idLen, verLen int) string {
	formatStr := fmt.Sprintf("%%-%ds   %%-%ds   %%-%ds -> %%s", nameLen, idLen, verLen)
	return fmt.Sprintf(formatStr, pkg.Name, pkg.Id, pkg.Version, pkg.AvailableVersion)
}
