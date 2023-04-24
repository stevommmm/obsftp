# ObSFTP

Object Store backed SFTP daemon.

Translation layer for SFTP to a backing object store. All paths are translated as `/<bucket/<blob_name>`.



Quirks:

- Directories are a funny thing in object world, in this implementation you can `cd nowhere` and it will pretend this directory exists.
- Clients need to `cd` or upload into one of the bucket root folders they can see. The software won't create new buckets for undefined top level folders.
- Partial/resumed uploads are just tricky, so have been left out for now, produces an error on attempt.
- SFTP Username and Password must both be longer than 16 characters.

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


## Policies

Access keys + Policies should be used to define access into specific buckets as required. e.g. to allow a client to perform any action on `test_bucket` we would use:

```json
{
 "Version": "2012-10-17",
 "Statement": [
  {
   "Effect": "Allow",
   "Action": [
    "s3:*"
   ],
   "Resource": [
    "arn:aws:s3:::test_bucket/*"
   ]
  }
 ]
}
```
