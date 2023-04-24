package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	cliBind           string
	cliVerbose        bool
	cliEndpoint       string
	ckiEndpointSecure bool
	config            *ssh.ServerConfig
)

func clientHandler(nConn net.Conn) {
	// Before use, a handshake must be performed on the incoming
	// net.Conn.
	sConn, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		log.Printf("%q failed to handshake\n", nConn.RemoteAddr(), err)
		nConn.Close()
		return
	}
	log.Printf("%q SSH server established\n", nConn.RemoteAddr())

	// Initialize minio client object.
	var client *BucketClient
	if c, err := minio.New(cliEndpoint, &minio.Options{
		Creds: credentials.NewStaticV4(
			sConn.Permissions.Extensions["OBJ_KEY"],
			sConn.Permissions.Extensions["OBJ_SECRET"],
			""),
		Secure: ckiEndpointSecure,
	}); err == nil {
		if cliVerbose {
			c.TraceOn(os.Stderr)
		}

		// Handles testing the credentials above are valid
		if authbuckets, err := c.ListBuckets(context.Background()); err != nil {
			sConn.Close()
			return
		} else if len(authbuckets) == 0 {
			sConn.Close()
			return
		}

		client = &BucketClient{
			client: c,
		}
	} else {
		log.Println(err)
		sConn.Close()
		return
	}

	// The incoming Request channel must be serviced.
	go ssh.DiscardRequests(reqs)

	// Service the incoming Channel channel.
	for newChannel := range chans {
		// Channels have a type, depending on the application level
		// protocol intended. In the case of an SFTP session, this is "subsystem"
		// with a payload string of "<length=4>sftp"
		log.Printf("%q Incoming channel: %s\n", nConn.RemoteAddr(), newChannel.ChannelType())
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			log.Printf("%q Unknown channel type: %s\n", nConn.RemoteAddr(), newChannel.ChannelType())
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("%q could not accept channel\n", nConn.RemoteAddr(), err)
			return
		}
		log.Printf("%q Channel accepted\n", nConn.RemoteAddr())

		// Sessions have out-of-band requests such as "shell",
		// "pty-req" and "env".  Here we handle only the
		// "subsystem" request.
		go func(in <-chan *ssh.Request) {
			for req := range in {
				log.Printf("%q Request: %v\n", nConn.RemoteAddr(), req.Type)
				ok := false
				switch req.Type {
				case "subsystem":
					log.Printf("%q Subsystem: %s\n", nConn.RemoteAddr(), req.Payload[4:])
					if string(req.Payload[4:]) == "sftp" {
						ok = true
					}
				}
				log.Printf("%q - accepted: %v\n", nConn.RemoteAddr(), ok)
				req.Reply(ok, nil)
			}
		}(requests)

		serverOptions := []sftp.RequestServerOption{}
		log.Printf("%#v\n", client)
		server := sftp.NewRequestServer(
			channel,
			sftp.Handlers{
				FileGet:  client,
				FilePut:  client,
				FileCmd:  client,
				FileList: client,
			},
			serverOptions...,
		)
		if err := server.Serve(); err == io.EOF {
			server.Close()
			log.Printf("%q sftp client exited session\n", nConn.RemoteAddr())
		} else if err != nil {
			log.Printf("%q sftp server completed with error:", nConn.RemoteAddr(), err)
		}
	}
}

func main() {
	flag.StringVar(&cliBind, "bind", "0.0.0.0:2222", "SFTP server listen address")
	flag.BoolVar(&cliVerbose, "v", false, "Verbose mode")
	flag.StringVar(&cliEndpoint, "endpoint", "127.0.0.1:9000", "Remote Object Store location")
	flag.BoolVar(&ckiEndpointSecure, "secure-endpoint", false, "Remote Object Store uses SSL")
	flag.Parse()

	config = &ssh.ServerConfig{
		NoClientAuth: false,
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if len(c.User()) < 16 || len(pass) < 16 {
				return nil, fmt.Errorf("Invalid login.")
			}
			// Used later on in S3 connections
			return &ssh.Permissions{Extensions: map[string]string{"OBJ_KEY": c.User(), "OBJ_SECRET": string(pass)}}, nil
		},
	}

	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatal("Failed to generate keys", err)
	}

	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		log.Fatal("signer from key", err)
	}
	config.AddHostKey(signer)

	// Once a ServerConfig has been configured, connections can be
	// accepted.
	listener, err := net.Listen("tcp4", cliBind)
	if err != nil {
		log.Fatal("failed to listen for connection", err)
	}
	log.Printf("Listening on %v\n", listener.Addr())

	for {
		nConn, err := listener.Accept()
		if err != nil {
			log.Printf("%q failed to accept incoming connection", nConn.RemoteAddr(), err)
			continue
		}
		go clientHandler(nConn)
	}
}
