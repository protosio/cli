#!/bin/bash

VERSION=$1

if [[ $VERSION == "" ]]; then
  echo "Specify a version"
  exit 1
fi

# macOS
env GOOS=darwin GOARCH=amd64 go build -o ./bin/protos-cli_${VERSION}_darwin cmd/protos/*.go

# Linux
env GOOS=linux GOARCH=amd64 go build -o ./bin/protos-cli_${VERSION}_linux cmd/protos/*.go

# Windows
env GOOS=windows GOARCH=amd64 go build -o ./bin/protos-cli_${VERSION}_windows cmd/protos/*.go
