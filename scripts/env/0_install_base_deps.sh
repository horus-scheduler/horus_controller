#!/bin/bash

GO_VERSION=1.18.1
GO_URL=https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz


wget --inet4-only ${GO_URL}
rm -rf /usr/local/go && tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz

rm go${GO_VERSION}.linux-amd64.tar.gz
