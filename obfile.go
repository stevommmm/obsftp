package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

type obf struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func FileStatFromObjectInfo(stat *minio.ObjectInfo) obf {
	mode := fs.FileMode(0750)
	if stat.ETag == "" {
		mode = mode | fs.ModeDir
	}
	// Folders dont really have a time, so they show as negative
	mod := stat.LastModified
	if stat.LastModified.Unix() <= 0 {
		mod = time.Now()
	}
	// Forge 'folders' by trimming out the search prefix
	return obf{
		name:    filepath.Base(stat.Key),
		size:    stat.Size,
		mode:    mode,
		modTime: mod,
		isDir:   stat.ETag == "",
	}
}

func FileStatForDir(path string) obf {
	return obf{
		name:    path,
		size:    0,
		mode:    fs.FileMode(0750) | fs.ModeDir,
		modTime: time.Now(),
		isDir:   true,
	}
}

// base name of the file
func (o obf) Name() string {
	return strings.TrimSuffix(o.name, "/")
}

// length in bytes for regular files; system-dependent for others
func (o obf) Size() int64 {
	return o.size
}

// file mode bits
func (o obf) Mode() os.FileMode {
	return o.mode
}

// modification time
func (o obf) ModTime() time.Time {
	return o.modTime
}

// abbreviation for Mode().IsDir()
func (o obf) IsDir() bool {
	return o.mode.IsDir()
}

// underlying data source (can return nil)
func (o obf) Sys() any {
	return nil
}
