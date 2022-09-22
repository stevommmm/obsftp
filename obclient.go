package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/pkg/sftp"
)

type obc struct {
	client *minio.Client
	name   string
}

func (o obc) IsValidUser(name string) bool {
	ok, err := o.client.BucketExists(context.Background(), name)
	if ok && err == nil {
		return true
	}
	return false
}

func (o obc) For(name string) *obc {
	if o.IsValidUser(name) {
		return &obc{o.client, name}
	}
	return nil
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

func (c obc) Filelist(req *sftp.Request) (sftp.ListerAt, error) {
	files := FileListAt{}
	// Remove leading / from everything
	path := strings.TrimPrefix(filepath.Clean(req.Filepath), "/")
	log.Printf("\nFilelist %q: %q %q\n", c.name, req.Method, path)
	switch req.Method {
	case "Stat":
		obs, err := c.client.StatObject(
			context.Background(),
			c.name,
			path,
			minio.StatObjectOptions{},
		)
		if err == nil {
			files = append(files, FileStatFromObjectInfo(&obs))
		} else {
			files = append(files, FileStatForDir(path))
		}
	case "List":
		for obs := range c.client.ListObjects(
			context.Background(),
			c.name,
			minio.ListObjectsOptions{Prefix: path + "/"},
		) {
			files = append(files, FileStatFromObjectInfo(&obs))
		}
	}
	return files, nil
}

// Functions to send back nice names for owners
func (c obc) LookupUserName(_ string) string {
	return c.name
}
func (c obc) LookupGroupName(_ string) string {
	return c.name
}

func (c obc) Fileread(req *sftp.Request) (io.ReaderAt, error) {
	path := strings.TrimPrefix(filepath.Clean(req.Filepath), "/")
	log.Println("Fileread:", path, req)
	obs, err := c.client.GetObject(
		req.Context(),
		c.name,
		path,
		minio.GetObjectOptions{},
	)
	b, err := io.ReadAll(obs)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

func (o obc) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	path := strings.TrimPrefix(filepath.Clean(req.Filepath), "/")
	log.Printf("Filewrite: %#v\n", req)
	return FileWriteAt{o, path}, nil
}

type FileWriteAt struct {
	o    obc
	path string
}

func (fwa FileWriteAt) WriteAt(p []byte, off int64) (int, error) {
	b := bytes.Buffer{}
	if off > 0 {
		return 0, fmt.Errorf("Offset writes not supported")
	}
	_, err := b.Write(p)
	if err != nil {
		return 0, err
	}
	i, err := fwa.o.client.PutObject(context.Background(), fwa.o.name, fwa.path, &b, int64(b.Len()), minio.PutObjectOptions{})
	if err != nil {
		return 0, err
	}
	return int(i.Size), nil
}

func (o obc) Filecmd(req *sftp.Request) error {
	log.Printf("Filecmd: %#v\n", req)
	return nil
}
