package main

import (
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v6"
)

type obf struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func FileStatFromObjectInfo(stat *minio.ObjectInfo) *obf {
	mode := fs.ModePerm
	if stat.ETag == "" {
		mode = mode | fs.ModeDir
	}
	return &obf{
		name:    stat.Key,
		size:    stat.Size,
		mode:    mode,
		modTime: stat.LastModified,
		isDir:   stat.ETag == "",
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
