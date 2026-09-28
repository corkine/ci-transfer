package transfer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTestFile(t *testing.T, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestTransferPaths(t *testing.T) {
	root := t.TempDir()
	file := writeTestFile(t, filepath.Join(root, "package.zip"), "zip")
	dir := filepath.Join(root, "dist")
	writeTestFile(t, filepath.Join(dir, "nested", "app.txt"), "app")
	for _, tc := range []struct {
		name        string
		sources     []string
		destination string
		oss         bool
		want        []string
	}{
		{"OSS file exact", []string{file}, "/service/release.zip", true, []string{"service/release.zip"}},
		{"OSS file directory", []string{file}, "/service/", true, []string{"service/package.zip"}},
		{"OSS contents", []string{dir}, "/service/", true, []string{"service/nested/app.txt"}},
		{"OSS multiple", []string{file, dir}, "/service", true, []string{"service/package.zip", "service/nested/app.txt"}},
		{"SSH exact", []string{file}, "/deploy/trigger", false, []string{"/deploy/trigger"}},
		{"SSH directory", []string{dir}, "/deploy/", false, []string{"/deploy/dist/nested/app.txt"}},
		{"SSH directory exact", []string{dir}, "/deploy", false, []string{"/deploy/nested/app.txt"}},
		{"SSH multiple", []string{file, dir}, "/deploy", false, []string{"/deploy/package.zip", "/deploy/dist/nested/app.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := planTransfers(tc.sources, tc.destination, tc.oss)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range plan.Files {
				got = append(got, f.Remote)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
