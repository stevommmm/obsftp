package main

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/minio/minio-go/v7"
)

type ObjectFile struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool

	lock sync.Mutex

	ob_content   []byte
	ob_conn      *minio.Client
	ob_bucket    string
	ob_direction string
}

func ObjectFileFromObjectInfo(stat *minio.ObjectInfo) *ObjectFile {
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
	return &ObjectFile{
		name:    filepath.Base(stat.Key),
		size:    stat.Size,
		mode:    mode,
		modTime: mod,
		isDir:   stat.ETag == "",
	}
}

func ObjectFileEmptyDir(path string) *ObjectFile {
	return &ObjectFile{
		name:    path,
		size:    0,
		mode:    fs.FileMode(0750) | fs.ModeDir,
		modTime: time.Now(),
		isDir:   true,
	}
}

func (o *ObjectFile) FetchStat() error {
	o.lock.Lock()
	defer o.lock.Unlock()
	stat, err := o.ob_conn.StatObject(
		context.Background(),
		o.ob_bucket,
		o.Name(),
		minio.StatObjectOptions{},
	)
	if err != nil {
		return err
	}

	o.mode = fs.FileMode(0750)
	if stat.ETag == "" {
		o.mode = o.mode | fs.ModeDir
	}
	// Folders dont really have a time, so they show as negative
	o.modTime = stat.LastModified
	if stat.LastModified.Unix() <= 0 {
		o.modTime = time.Now()
	}
	// Forge 'folders' by trimming out the search prefix
	o.name = filepath.Base(stat.Key)
	o.size = stat.Size
	o.isDir = stat.ETag == ""
	return nil
}

func (o *ObjectFile) FetchContent() (err error) {
	o.lock.Lock()
	defer o.lock.Unlock()
	obs, err := o.ob_conn.GetObject(
		context.Background(),
		o.ob_bucket,
		o.Name(),
		minio.GetObjectOptions{},
	)
	if err != nil {
		return os.ErrNotExist
	}
	o.ob_content, err = io.ReadAll(obs)
	o.size = int64(len(o.ob_content))
	if err != nil {
		return err
	}
	return nil
}

//
// SFTP server interface implementation
//

// base name of the file
func (o *ObjectFile) Name() string {
	return strings.TrimSuffix(o.name, "/")
}

// length in bytes for regular files; system-dependent for others
func (o *ObjectFile) Size() int64 {
	return o.size
}

// Return actual buffer size
func (o *ObjectFile) bSize() int64 {
	return int64(len(o.ob_content))
}

// file mode bits
func (o *ObjectFile) Mode() os.FileMode {
	return o.mode
}

// modification time
func (o *ObjectFile) ModTime() time.Time {
	return o.modTime
}

// abbreviation for Mode().IsDir()
func (o *ObjectFile) IsDir() bool {
	return o.mode.IsDir()
}

// underlying data source (can return nil)
func (o *ObjectFile) Sys() any {
	return &syscall.Stat_t{
		Uid: 65534,
		Gid: 65534,
	}
}

// File reader implementation
func (o *ObjectFile) ReadAt(b []byte, off int64) (int, error) {
	o.lock.Lock()
	defer o.lock.Unlock()
	// Check that our value is smaller than *int will allow
	if off < 0 || int64(int(off)) < off {
		return 0, fs.ErrInvalid
	}
	if off > int64(len(o.ob_content)) {
		return 0, io.EOF
	}
	n := copy(b, o.ob_content[off:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

// File writer implementation
func (o *ObjectFile) WriteAt(p []byte, off int64) (int, error) {
	o.lock.Lock()
	defer o.lock.Unlock()
	grow := int64(len(p)) + off - o.bSize()
	if grow > 0 {
		o.ob_content = append(o.ob_content, make([]byte, grow)...)
	}
	return copy(o.ob_content[off:], p), nil
}
func (o *ObjectFile) Close() error {
	o.lock.Lock()
	defer o.lock.Unlock()
	switch o.ob_direction {
	case "upload":
		n, err := o.ob_conn.PutObject(
			context.Background(),
			o.ob_bucket,
			strings.TrimPrefix(o.Name(), "/"),
			bytes.NewReader(o.ob_content),
			o.bSize(),
			minio.PutObjectOptions{},
		)
		if err != nil {
			log.Println("!PutObject", o.Name(), err)
			return err
		}
		if n.Size == 0 {
			log.Println("!?PutObject", o.Name(), "0 byte upload")
		}
	}
	o.ob_content = make([]byte, 1)
	return nil
}
