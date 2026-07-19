package dependency

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	projectarchive "github.com/billstark001/latexmk/packages/cli/internal/archive"
)

func TestPaperExamplesAreExecutableDependencyFixtures(t *testing.T) {
	repoRoot := findRepositoryRoot(t)
	tests := []struct {
		name string
		want []string
	}{
		{name: "slim", want: []string{
			"figures/remote-compilation.png",
			"main.tex",
			"references.bib",
			"sections/introduction.tex",
			"sections/method.tex",
			"sections/results.tex",
		}},
		{name: "ieee", want: []string{
			"code/procedure.txt",
			"figures/remote-compilation.png",
			"main.tex",
			"references.bib",
			"sections/introduction.tex",
			"sections/method.tex",
			"sections/results.tex",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(repoRoot, "examples", test.name)
			candidates, _, err := projectarchive.Manifest(projectarchive.Options{
				Root:             root,
				RespectGitIgnore: true,
				MaxFiles:         100,
				MaxBytes:         16 << 20,
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := Discover("main.tex", candidates)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Resolved {
				t.Fatalf("example has unresolved dependencies: %#v", result.Diagnostics)
			}
			got := make([]string, 0, len(result.Files))
			for _, file := range result.Files {
				got = append(got, file.Path)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("dependency closure = %#v, want %#v", got, test.want)
			}
		})
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "examples")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repository root from %s", dir)
		}
		dir = parent
	}
}
