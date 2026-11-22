package authored_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

var relativeShapes = map[string]string{
	"a directory":    filepath.Join("db", "finwise.db"),
	"dot slash":      "." + string(filepath.Separator) + filepath.Join("db", "finwise.db"),
	"working folder": "finwise.db",
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

// treeOf lists every file under root, relative to it, so that a test can say
// where a database was created and that it was created nowhere else.
func treeOf(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		files = append(files, relative)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	return files
}

func TestARelativePathCreatesTheDatabaseUnderTheWorkingDirectoryAndRecognisesIt(t *testing.T) {
	for name, relative := range relativeShapes {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			mustMkdir(t, filepath.Join(directory, "db"))
			t.Chdir(directory)

			openAndClose(t, relative, creationInstant, nil)
			want := []string{filepath.Clean(relative)}
			if got := treeOf(t, directory); !slices.Equal(got, want) {
				t.Fatalf("the first open left %v, want %v", got, want)
			}
			if got := headerID(t, filepath.Join(directory, relative)); got != ourID {
				t.Errorf("application id %s, want %s", got, ourID)
			}

			second, err := authored.Open(t.Context(), relative, authored.Options{Now: clockAt(t, laterInstant)})
			if err != nil {
				t.Fatalf("second open: %v", err)
			}
			if got := trackingStart(t, second); got != "2026-11" {
				t.Errorf("tracking start month = %s after the second open, want the month of the first", got)
			}
			if err := second.Close(); err != nil {
				t.Fatal(err)
			}
			if got := treeOf(t, directory); !slices.Equal(got, want) {
				t.Errorf("the second open left %v", got)
			}
		})
	}
}

func TestARelativePathToAForeignFileIsRefusedAsItsAbsolutePathIs(t *testing.T) {
	foreign := map[string]func(t *testing.T, path string){
		"text": func(t *testing.T, path string) {
			writeBytes(t, path, "a note, not a database, long enough to be read as one\n")
		},
		"foreign SQLite": func(t *testing.T, path string) {
			rawDatabase(t, path, "CREATE TABLE notes (body TEXT)", "INSERT INTO notes VALUES ('x')")
		},
		"short, not SQLite": func(t *testing.T, path string) { writeBytes(t, path, "ten bytes!") },
	}
	for kind, build := range foreign {
		for shape, relative := range relativeShapes {
			t.Run(kind+", "+shape, func(t *testing.T) {
				directory := t.TempDir()
				mustMkdir(t, filepath.Join(directory, "db"))
				absolute := filepath.Join(directory, relative)
				build(t, absolute)
				t.Chdir(directory)
				hashBefore, filesBefore := fileHash(t, absolute), treeOf(t, directory)

				_, relativeErr := authored.Open(t.Context(), relative, authored.Options{Now: clockAt(t, creationInstant)})
				_, absoluteErr := authored.Open(t.Context(), absolute, authored.Options{Now: clockAt(t, creationInstant)})

				if got := asOpenError(t, relativeErr); got.Failure != authored.FailureNotOurs || got.Path != relative {
					t.Errorf("relative: failure %d at %q, want FailureNotOurs at the path as given", got.Failure, got.Path)
				}
				if got := asOpenError(t, absoluteErr); got.Failure != authored.FailureNotOurs {
					t.Errorf("absolute: failure %d, want FailureNotOurs", got.Failure)
				}
				if hashAfter := fileHash(t, absolute); hashAfter != hashBefore {
					t.Errorf("the file changed: %s -> %s", hashBefore, hashAfter)
				}
				if filesAfter := treeOf(t, directory); !slices.Equal(filesBefore, filesAfter) {
					t.Errorf("the tree changed: %v -> %v", filesBefore, filesAfter)
				}
			})
		}
	}
}

func writeBytes(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestADirectoryWithURICharactersHoldsTheDatabaseAbsolutelyAndRelatively(t *testing.T) {
	const awkward = "a b#c?d%e"
	parent := t.TempDir()
	mustMkdir(t, filepath.Join(parent, awkward))
	absolute := filepath.Join(parent, awkward, "finwise.db")

	openAndClose(t, absolute, creationInstant, nil)
	if got := treeOf(t, parent); !slices.Equal(got, []string{filepath.Join(awkward, "finwise.db")}) {
		t.Fatalf("the absolute open left %v", got)
	}

	t.Chdir(parent)
	relative := filepath.Join(awkward, "finwise.db")
	reopened, err := authored.Open(t.Context(), relative, authored.Options{Now: clockAt(t, laterInstant)})
	if err != nil {
		t.Fatalf("relative open: %v", err)
	}
	if got := trackingStart(t, reopened); got != "2026-11" {
		t.Errorf("tracking start month = %s through the relative path", got)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if got := treeOf(t, parent); !slices.Equal(got, []string{relative}) {
		t.Errorf("the relative open left %v", got)
	}
}

func TestARelativePathWhoseDirectoryIsMissingIsTheErrorOfAnAbsoluteOne(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	_, err := authored.Open(t.Context(), filepath.Join("absent", "finwise.db"), authored.Options{Now: clockAt(t, creationInstant)})
	if got := asOpenError(t, err); got.Failure != authored.FailureMissingDirectory {
		t.Errorf("failure %d, want FailureMissingDirectory", got.Failure)
	}
	if files := listDirectory(t, directory); len(files) != 0 {
		t.Errorf("a directory was created: %v", files)
	}
}
