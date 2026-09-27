package layerfs

import (
	"errors"
	"reflect"
	"testing"
)

func TestLowerLayerPriorityAndCopyOnWrite(t *testing.T) {
	fs := New(
		Layer{"etc/config": []byte("first"), "shared": []byte("high")},
		Layer{"etc/config": []byte("second"), "readme": []byte("lower")},
	)

	got, err := fs.ReadFile("/etc/config")
	if err != nil || string(got) != "first" {
		t.Fatalf("ReadFile() = %q, %v; want first", got, err)
	}
	if err := fs.WriteFile("etc/config", []byte("changed")); err != nil {
		t.Fatal(err)
	}
	got, err = fs.ReadFile("etc/config")
	if err != nil || string(got) != "changed" {
		t.Fatalf("ReadFile() after write = %q, %v; want changed", got, err)
	}
	if !reflect.DeepEqual(fs.UpperFiles(), []string{"/etc/config"}) {
		t.Fatalf("UpperFiles() = %v", fs.UpperFiles())
	}
}

func TestRemoveCreatesWhiteoutAndHidesDirectoryTree(t *testing.T) {
	fs := New(Layer{"docs/guide.txt": []byte("guide"), "docs/nested/info.txt": []byte("info")})
	if err := fs.Remove("/docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile("docs/guide.txt"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("ReadFile() error = %v; want ErrNotExist", err)
	}
	if !reflect.DeepEqual(fs.VisibleFiles(), []string{}) {
		t.Fatalf("VisibleFiles() = %v; want empty", fs.VisibleFiles())
	}
	if !reflect.DeepEqual(fs.Whiteouts(), []string{"/docs"}) {
		t.Fatalf("Whiteouts() = %v", fs.Whiteouts())
	}
	if err := fs.WriteFile("docs/new.txt", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fs.VisibleFiles(), []string{"/docs/new.txt"}) {
		t.Fatalf("VisibleFiles() after recreating path = %v", fs.VisibleFiles())
	}
}

func TestReadDirMergesAndSortsEntries(t *testing.T) {
	fs := New(Layer{"a.txt": []byte("a"), "dir/from-lower.txt": []byte("lower")})
	if err := fs.WriteFile("dir/from-upper.txt", []byte("upper")); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir("/")
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{Name: "a.txt", Size: 1}, {Name: "dir", IsDir: true}}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("ReadDir(/) = %#v; want %#v", entries, want)
	}
	entries, err = fs.ReadDir("dir")
	if err != nil {
		t.Fatal(err)
	}
	want = []Entry{{Name: "from-lower.txt", Size: 5}, {Name: "from-upper.txt", Size: 5}}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("ReadDir(dir) = %#v; want %#v", entries, want)
	}
}

func TestRejectsTraversalAndFileDirectoryConflicts(t *testing.T) {
	fs := New(Layer{"file.txt": []byte("file"), "dir/child.txt": []byte("child")})
	if err := fs.WriteFile("../../escape", []byte("no")); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("WriteFile traversal error = %v; want ErrInvalidPath", err)
	}
	if err := fs.WriteFile("file.txt/child", []byte("no")); !errors.Is(err, ErrIsFile) {
		t.Fatalf("WriteFile under file error = %v; want ErrIsFile", err)
	}
	if err := fs.WriteFile("dir", []byte("no")); !errors.Is(err, ErrIsDirectory) {
		t.Fatalf("WriteFile over directory error = %v; want ErrIsDirectory", err)
	}
}

func TestReturnedBytesDoNotMutateFilesystem(t *testing.T) {
	fs := New(Layer{"file": []byte("original")})
	data, err := fs.ReadFile("file")
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	got, err := fs.ReadFile("file")
	if err != nil || string(got) != "original" {
		t.Fatalf("ReadFile() after mutating result = %q, %v", got, err)
	}
}
