#!/bin/sh
set -e
set -u

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o ./dash-zmqproxy-linux-x86_64 ./cmd/dash-zmqproxy/

# scp -rp ./dash-zmqproxy-linux-x86_64 tdash-zmq:~/bin/dash-zmqproxy
