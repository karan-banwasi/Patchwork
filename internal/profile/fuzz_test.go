package profile

import "testing"

func FuzzParsePatchfile(f *testing.F) {
	seeds := [][]byte{
		[]byte("# Patchwork machine profile\nwinget \"Git.Git\"\nscoop \"jq\" version=\"1.7.1\"\npin \"npm:typescript\" version=\"<6.0.0\" reason=\"hold\"\n"),
		[]byte(`{"schema_version":"1.0.0","packages":[{"manager":"npm","id":"typescript","version":"5.7.2"}],"pins":[{"id":"Git.Git"}]}`),
		[]byte(`{"schema_version":"1.0.0","packages":[`),
		[]byte("pin \"unterminated\n"),
		[]byte("# comment only\n\n"),
		[]byte{},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			return
		}

		prof, err := ParsePatchfile(data)
		if err == nil {
			_ = FormatPatchfile(prof)
			_, _ = MarshalJSON(prof)
		}
	})
}
