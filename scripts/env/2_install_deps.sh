#!/bin/bash


# go get -u github.com/golang/protobuf/{proto,protoc-gen-go}
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2