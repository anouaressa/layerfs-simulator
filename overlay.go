// Package layerfs provides an in-memory simulator for a layered, copy-on-write
// filesystem inspired by OverlayFS.
package layerfs

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
)

var (
	ErrNotExist    = errors.New("layerfs: file or directory does not exist")
	ErrIsDirectory = errors.New("layerfs: path is a directory")
	ErrIsFile      = errors.New("layerfs: path is a file")
	ErrInvalidPath = errors.New("layerfs: invalid path")
)

// Layer represents one immutable lower layer. File keys are slash-separated
// paths, with or without a leading slash. New copies the layer contents.
type Layer map[string][]byte

// Entry describes an immediate child returned by ReadDir.
type Entry struct {
	Name  string
	IsDir bool
	Size  int
}

// FS combines immutable lower layers with a writable upper layer. Lower layers
// are ordered from highest to lowest priority. Files written through FS live in
// the upper layer and shadow files with the same path in lower layers.
type FS struct {
	lower     []map[string][]byte
	upper     map[string][]byte
	whiteouts map[string]struct{}
}

// New creates a filesystem from lower layers. Earlier layers take precedence
// over later layers when a path occurs more than once.
func New(layers ...Layer) *FS {
	fs := &FS{
		lower:     make([]map[string][]byte, 0, len(layers)),
		upper:     make(map[string][]byte),
		whiteouts: make(map[string]struct{}),
	}
	for _, layer := range layers {
		copyOfLayer := make(map[string][]byte, len(layer))
		for rawPath, contents := range layer {
			cleanPath, err := normalize(rawPath)
			if err == nil {
				copyOfLayer[cleanPath] = clone(contents)
			}
		}
		fs.lower = append(fs.lower, copyOfLayer)
	}
	return fs
}

// ReadFile returns the visible contents of a file. The returned bytes are a
// copy, so callers cannot mutate filesystem contents without WriteFile.
func (fs *FS) ReadFile(name string) ([]byte, error) {
	cleanPath, err := normalize(name)
	if err != nil {
		return nil, err
	}
	if data, ok := fs.upper[cleanPath]; ok {
		return clone(data), nil
	}
	if fs.isDirectory(cleanPath) {
		return nil, fmt.Errorf("%w: %s", ErrIsDirectory, cleanPath)
	}
	if fs.isHidden(cleanPath) {
		return nil, fmt.Errorf("%w: %s", ErrNotExist, cleanPath)
	}
	for _, layer := range fs.lower {
		if data, ok := layer[cleanPath]; ok {
			return clone(data), nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotExist, cleanPath)
}

// WriteFile creates or replaces a file in the writable upper layer.
func (fs *FS) WriteFile(name string, data []byte) error {
	cleanPath, err := normalize(name)
	if err != nil {
		return err
	}
	if fs.isDirectory(cleanPath) {
		return fmt.Errorf("%w: %s", ErrIsDirectory, cleanPath)
	}
	if fs.hasFileDescendant(cleanPath) {
		return fmt.Errorf("%w: %s has file descendants", ErrIsFile, cleanPath)
	}
	if err := fs.ensureParentDirectories(cleanPath); err != nil {
		return err
	}
	delete(fs.whiteouts, cleanPath)
	fs.upper[cleanPath] = clone(data)
	return nil
}

// Remove hides a file or directory from the merged view. Removing a lower
// layer entry creates a whiteout in the upper layer. Removing a directory
// hides its complete subtree.
func (fs *FS) Remove(name string) error {
	cleanPath, err := normalize(name)
	if err != nil {
		return err
	}
	if !fs.exists(cleanPath) {
		return fmt.Errorf("%w: %s", ErrNotExist, cleanPath)
	}
	for file := range fs.upper {
		if file == cleanPath || isWithin(file, cleanPath) {
			delete(fs.upper, file)
		}
	}
	for whiteout := range fs.whiteouts {
		if whiteout == cleanPath || isWithin(whiteout, cleanPath) {
			delete(fs.whiteouts, whiteout)
		}
	}
	fs.whiteouts[cleanPath] = struct{}{}
	return nil
}

// ReadDir returns the sorted, immediate children of a directory in the merged
// filesystem view. Directories are inferred from file paths; empty directories
// are not represented by this simulator.
func (fs *FS) ReadDir(name string) ([]Entry, error) {
	cleanPath, err := normalizeDirectory(name)
	if err != nil {
		return nil, err
	}
	if cleanPath != "" && !fs.isDirectory(cleanPath) {
		if fs.hasVisibleFile(cleanPath) {
			return nil, fmt.Errorf("%w: %s", ErrIsFile, cleanPath)
		}
		return nil, fmt.Errorf("%w: %s", ErrNotExist, cleanPath)
	}
	children := make(map[string]Entry)
	for _, file := range fs.visibleFiles() {
		if cleanPath != "" && !isWithin(file, cleanPath) {
			continue
		}
		remainder := file
		if cleanPath != "" {
			remainder = strings.TrimPrefix(file, cleanPath+"/")
		}
		parts := strings.SplitN(remainder, "/", 2)
		entry := Entry{Name: parts[0]}
		if len(parts) == 2 {
			entry.IsDir = true
		} else {
			entry.Size = len(fs.visibleData(file))
		}
		if old, exists := children[entry.Name]; exists && old.IsDir {
			continue
		}
		children[entry.Name] = entry
	}
	entries := make([]Entry, 0, len(children))
	for _, entry := range children {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// VisibleFiles returns all visible file paths, sorted and rooted with '/'.
func (fs *FS) VisibleFiles() []string {
	files := fs.visibleFiles()
	for i := range files {
		files[i] = "/" + files[i]
	}
	return files
}

// UpperFiles returns sorted paths currently stored in the writable layer.
func (fs *FS) UpperFiles() []string {
	files := make([]string, 0, len(fs.upper))
	for name := range fs.upper {
		files = append(files, "/"+name)
	}
	sort.Strings(files)
	return files
}

// Whiteouts returns sorted paths hidden from lower layers by removals.
func (fs *FS) Whiteouts() []string {
	paths := make([]string, 0, len(fs.whiteouts))
	for name := range fs.whiteouts {
		paths = append(paths, "/"+name)
	}
	sort.Strings(paths)
	return paths
}

func (fs *FS) visibleFiles() []string {
	all := make(map[string]struct{})
	for _, layer := range fs.lower {
		for name := range layer {
			all[name] = struct{}{}
		}
	}
	for name := range fs.upper {
		all[name] = struct{}{}
	}
	files := make([]string, 0, len(all))
	for name := range all {
		if fs.hasVisibleFile(name) {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	return files
}

func (fs *FS) hasVisibleFile(name string) bool {
	if _, ok := fs.upper[name]; ok {
		return true
	}
	if fs.isHidden(name) {
		return false
	}
	for _, layer := range fs.lower {
		if _, ok := layer[name]; ok {
			return true
		}
	}
	return false
}

func (fs *FS) visibleData(name string) []byte {
	if data, ok := fs.upper[name]; ok {
		return data
	}
	for _, layer := range fs.lower {
		if data, ok := layer[name]; ok && !fs.isHidden(name) {
			return data
		}
	}
	return nil
}

func (fs *FS) exists(name string) bool {
	return fs.hasVisibleFile(name) || fs.isDirectory(name)
}

func (fs *FS) isDirectory(name string) bool {
	for _, file := range fs.visibleFiles() {
		if isWithin(file, name) {
			return true
		}
	}
	return false
}

func (fs *FS) hasFileDescendant(name string) bool {
	for file := range fs.upper {
		if isWithin(file, name) {
			return true
		}
	}
	for _, layer := range fs.lower {
		for file := range layer {
			if isWithin(file, name) && !fs.isHidden(file) {
				return true
			}
		}
	}
	return false
}

func (fs *FS) ensureParentDirectories(name string) error {
	parent := path.Dir(name)
	for parent != "." && parent != "/" {
		if fs.hasVisibleFile(parent) {
			return fmt.Errorf("%w: parent %s", ErrIsFile, parent)
		}
		parent = path.Dir(parent)
	}
	return nil
}

func (fs *FS) isHidden(name string) bool {
	for whiteout := range fs.whiteouts {
		if name == whiteout || isWithin(name, whiteout) {
			return true
		}
	}
	return false
}

func normalizeDirectory(name string) (string, error) {
	if name == "" || name == "/" || name == "." {
		return "", nil
	}
	return normalize(name)
}

func normalize(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	clean := path.Clean(strings.TrimPrefix(name, "/"))
	if clean == "." || clean == "" || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", ErrInvalidPath, name)
	}
	return clean, nil
}

func isWithin(name, directory string) bool {
	return directory != "" && strings.HasPrefix(name, directory+"/")
}

func clone(data []byte) []byte {
	return append([]byte(nil), data...)
}
