package main

import (
	"io"
	"io/fs"
	"log"
	"os"
	// "strings"

	"github.com/minio/minio-go/v6"
	"github.com/pkg/sftp"
)

type obc struct {
	client *minio.Client
	name   string
}

func (o obc) IsValidUser(name string) bool {
	ok, err := o.client.BucketExists(name)
	log.Println(ok, err)
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

type listerat []os.FileInfo

// Modeled after strings.Reader's ReadAt() implementation
func (f listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
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
	files := listerat{}
	log.Printf("Filelist %q: %#v\n", c.name, req)
	switch req.Method {
	case "Stat":
		stats, err := c.client.GetObjectACL(c.name, req.Filepath,)
		log.Println("Stat", stats, err)
		if err != nil {
			return files, err
		}
		mode := fs.ModePerm
		if stats.ETag == "" {
			mode = mode | fs.ModeDir
		}
		files = append(files, FileStatFromObjectInfo(stats))
	case "List":
		doneCh := make(chan struct{})
		defer close(doneCh)
		for obs := range c.client.ListObjectsV2(c.name, req.Filepath, false, doneCh) {
			log.Printf("%#v\n", obs)
			mode := fs.ModePerm
			if obs.ETag == "" {
				mode = mode | fs.ModeDir
			}
			files = append(files, FileStatFromObjectInfo(&obs))
		}
	}
	// 	fileList, err := client.ReadDir(req.Filepath)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return listerat(fileList), nil
	// case "Stat":
	// case "Readlink":
	// }
	return files, nil
}

func (c obc) Fileread(req *sftp.Request) (io.ReaderAt, error) {
	log.Printf("Fileread: %#v\n", req)
	return nil, nil
}

func (o obc) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	log.Printf("Filewrite: %#v\n", req)
	return nil, nil
}

func (o obc) Filecmd(req *sftp.Request) error {
	log.Printf("Filecmd: %#v\n", req)
	return nil
}
