package winget

import (
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func FuzzParseWingetOutput(f *testing.F) {
	seeds := []string{
		"Name               Id                        Version  Available Source\n------------------------------------------------------------------------\nGit                Git.Git                   2.44.0   2.45.0    winget\nWindows Terminal   Microsoft.WindowsTerminal 1.19.105 1.20.112  winget\n1 upgrades available.\n",
		"\x1b[?25l-\r \rName   Id   Version Available Source\n------------------------------------\nFoo    Foo.Bar 1.0  2.0 winget\n",
		"名前               Id        Version Available Source\n------------------------------------------------------\n日本語アプリ        App.CJK   1.0.0   1.1.0     winget\n",
		"Name   Id   Version Available\n-----------------------------\nName   Id   Version Available\n",
		"No installed package found matching input criteria.\n",
		"",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, output string) {
		if len(output) > 65536 {
			return
		}

		updates, err := parseWingetOutput(output)
		if err != nil {
			return
		}
		for _, u := range updates {
			if u.Id == "" {
				t.Fatalf("parseWingetOutput returned update with empty Id: %+v", u)
			}
			if u.Name == "" {
				t.Fatalf("parseWingetOutput returned update with empty Name: %+v", u)
			}
			if u.ManagerName != "winget" {
				t.Fatalf("parseWingetOutput returned ManagerName=%q, want winget", u.ManagerName)
			}
			if err := pm.ValidateWingetID(u.Id); err != nil {
				t.Fatalf("parseWingetOutput returned invalid Winget ID %q: %v", u.Id, err)
			}
		}
	})
}
