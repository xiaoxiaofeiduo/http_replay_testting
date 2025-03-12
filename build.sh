#! /bin/bash

rm -rf ./build/
mkdir ./build/

go build -o ./build/http_replay ./cmd
