package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/pkg/sftp"
)

var (
	GenericAuthError = fmt.Errorf("Bad Authentication")

	ObjectAuthorizedKeys = ".authorized_keys"
	ObjectAuthorizedPass = ".authorized_pass"
)

func normalizePath(path string) (string, error) {
	path = strings.TrimPrefix(filepath.Clean(path), "/")
	if path == ObjectAuthorizedKeys || path == ObjectAuthorizedPass {
		return "", fs.ErrPermission
	}
	return path, nil
}

type RootClient struct {
	client *minio.Client
}

type BucketClient struct {
	*RootClient
	name string
}

func (o *RootClient) hasBucket(user string) bool {
	ok, err := o.client.BucketExists(context.Background(), user)
	if ok && err == nil {
		return true
	}
	return false
}

func (o *RootClient) compareContent(user, heystack string, needle []byte) bool {
	f, err := o.client.GetObject(
		context.Background(),
		user,
		heystack,
		minio.GetObjectOptions{},
	)
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.Equal(line, needle) {
			return true
		}
	}
	return false
}

func (o *RootClient) ValidatePassword(user string, pass []byte) error {
	if !o.hasBucket(user) {
		return GenericAuthError
	}
	if !o.compareContent(user, ".authorized_pass", pass) {
		return GenericAuthError
	}
	return nil
}

func (o *RootClient) ValidatePublicKey(user string, authorized_key []byte) error {
	authorized_key = bytes.TrimSpace(authorized_key)
	if !o.hasBucket(user) {
		return GenericAuthError
	}
	if !o.compareContent(user, ".authorized_keys", authorized_key) {
		return GenericAuthError
	}
	return nil
}

func (o *RootClient) ForBucket(name string) *BucketClient {
	return &BucketClient{o, name}
}

type FileListAt []os.FileInfo

// copied from sftp in-memory handlers
func (f FileListAt) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	var n int
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}
	n = copy(ls, f[offset:])
	if n < len(ls) {
		return n, io.EOF
	}
	return n, nil
}

func (c *BucketClient) Filelist(req *sftp.Request) (sftp.ListerAt, error) {
	files := FileListAt{}
	path, err := normalizePath(req.Filepath)
	if err != nil {
		return files, err
	}
	log.Printf("\nFilelist %q: %q %q\n", c.name, req.Method, path)
	switch req.Method {
	case "Stat":
		obs := ObjectFile{
			ob_conn:   c.client,
			ob_bucket: c.name,
			name:      path,
		}
		if err := obs.FetchStat(); err == nil {
			files = append(files, &obs)
		} else {
			files = append(files, ObjectFileEmptyDir(path))
		}
		if len(files) == 0 {
			log.Println("No files, returning error")
			return files, os.ErrNotExist
		}
	case "List":
		for obs := range c.client.ListObjects(
			context.Background(),
			c.name,
			minio.ListObjectsOptions{Prefix: path + "/"},
		) {
			// Drop our config files out of directory listings
			if obs.Key == ObjectAuthorizedKeys || obs.Key == ObjectAuthorizedPass {
				continue
			}
			files = append(files, ObjectFileFromObjectInfo(&obs))
		}
	}
	return files, nil
}

// Functions to send back nice names for owners
func (c *BucketClient) LookupUserName(_ string) string {
	return c.name
}
func (c *BucketClient) LookupGroupName(_ string) string {
	return c.name
}

func (c *BucketClient) Fileread(req *sftp.Request) (io.ReaderAt, error) {
	path, err := normalizePath(req.Filepath)
	if err != nil {
		return nil, err
	}
	log.Println("Fileread:", path, req)
	obs := ObjectFile{
		ob_conn:   c.client,
		ob_bucket: c.name,
		name:      path,
	}
	if err := obs.FetchContent(); err == nil {
		return nil, err
	}

	return &obs, nil
}

func (o *BucketClient) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	path, err := normalizePath(req.Filepath)
	if err != nil {
		return nil, err
	}
	log.Printf("Filewrite: %#v\n", req)
	obs := ObjectFile{
		ob_conn:      o.client,
		ob_bucket:    o.name,
		name:         path,
		ob_direction: "upload",
	}
	obs.FetchStat()
	obs.FetchContent() // ignore errors n/a file etc
	return &obs, nil
}

func (c *BucketClient) Filecmd(req *sftp.Request) error {
	path, err := normalizePath(req.Filepath)
	if err != nil {
		return err
	}
	log.Printf("Filecmd: %#v\n %#v\n", req, *req.Attributes())
	switch req.Method {
	case "Mkdir":
		return nil
	case "Rename":
	case "Rmdir":
		lobs := c.client.ListObjects(
			context.Background(),
			c.name,
			minio.ListObjectsOptions{Prefix: path + "/", Recursive: true},
		)
		for _ = range c.client.RemoveObjects(context.Background(), c.name, lobs, minio.RemoveObjectsOptions{}) {
			return os.ErrInvalid
		}
		return nil
	case "Setstat":
		return nil
	case "Link", "Symlink":
		return os.ErrPermission
	case "Remove":
		return c.client.RemoveObject(context.Background(), c.name, path, minio.RemoveObjectOptions{})
	}

	return nil
}
