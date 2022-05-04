#!/bin/bash

mkdir -p ${HOME}/go/bin

echo 'export GOPATH=${HOME}/go' >> ~/.bashrc
echo 'export PATH=$PATH:/usr/local/go/bin:${HOME}/go/bin' >> ~/.bashrc

source ~/.bashrc
