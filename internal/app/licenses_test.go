package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Every module built into either platform's app has its licence in
// licenses/, which goes out with the app, and a line in its README.txt:
// a module added to go.mod and not there fails here.
func TestLicensesHaveEveryModule(t *testing.T) {
	root := filepath.Join("..", "..")
	readme, err := os.ReadFile(filepath.Join(root, "licenses", "README.txt"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(filepath.Join(root, "licenses"))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range [][2]string{{"darwin", "arm64"}, {"windows", "amd64"}} {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{with .Module}}{{if not .Main}}{{.Path}}{{end}}{{end}}", ".")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS="+target[0], "GOARCH="+target[1], "CGO_ENABLED=0")
		out, err := cmd.Output()
		if err != nil {
			t.Skipf("go list: %v", err)
		}
		for _, mod := range strings.Fields(string(out)) {
			if !strings.Contains(string(readme), mod+" ") {
				t.Errorf("%s/%s builds in %s, which licenses/README.txt does not name", target[0], target[1], mod)
			}
			file := strings.NewReplacer("/", "_").Replace(mod) + "-LICENSE"
			found := false
			for _, f := range files {
				found = found || strings.HasPrefix(f.Name(), file)
			}
			if !found {
				t.Errorf("no %s* in licenses/ for %s", file, mod)
			}
		}
	}
}
