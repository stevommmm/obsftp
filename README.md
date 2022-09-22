# ObSFTP

Object Store backed SFTP daemon.

Each sftp user translates to a bucket. Bucket existence defines access. Within the bucket there can be a `.passwords` and `.keys` file that is read and used to perform the 2 types of authorisation.

Querks:

- Directories are a funny thing in object world, in this implementation you can `cd nowhere` and it will pretend this directory exists. Useful for uploading files.
- Partial/resumed uploads are just tricky, so have been left out for now, produces an error on attempt.

## Setup

SFTP Server:
```bash
go run .
```
Object Store:
```bash
sudo podman run --rm -P docker.io/minio/minio server /mnt
```

Client:
```bash
sftp -o UserKnownHostsFile=/dev/null -P 2222 127.0.0.1
```



