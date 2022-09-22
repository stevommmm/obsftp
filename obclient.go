package main

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log"
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

func (o obc) list(prefix string, files *FileListAt) {
	log.Printf("list> %s:%s\n", o.name, prefix)
	for obs := range o.client.ListObjects(
		context.Background(),
		o.name,
		minio.ListObjectsOptions{Prefix: prefix},
	) {
		mode := fs.ModePerm
		if obs.ETag == "" {
			mode = mode | fs.ModeDir
		}
		*files = append(*files, FileStatFromObjectInfo(&obs))
	}
}

func (o obc) stat(name string, files *FileListAt) {
	log.Printf("stat> %q:%q\n", o.name, name)
	obs, err := o.client.StatObject(
		context.Background(),
		o.name,
		name,
		minio.StatObjectOptions{},
	)
	if err == nil {
		*files = append(*files, FileStatFromObjectInfo(&obs))
	} else {
		*files = append(*files, FileStatForDir(name))
	}
}

func (c obc) Filelist(req *sftp.Request) (sftp.ListerAt, error) {
	files := FileListAt{}
	// Remove leading / from everything
	path := strings.TrimPrefix(filepath.Clean(req.Filepath), "/")
	log.Printf("\nFilelist %q: %q %q\n", c.name, req.Method, path)
	switch req.Method {
	case "Stat":
		c.stat(path, &files)
	case "List":
		c.list(path+"/", &files)
	}
	log.Printf("<list %#v\n", files)
	return files, nil
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
	log.Printf("Filewrite: %#v\n", req)
	return nil, nil
}

func (o obc) Filecmd(req *sftp.Request) error {
	log.Printf("Filecmd: %#v\n", req)
	return nil
}
