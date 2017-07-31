#!/bin/bash

go get -d ./...
test dockerize.json -nt jsonfiles.go && GOOS=linux go generate
rm -f dockerize dockerize.linux dockerize.darwin dockerize.exe
GOOS=linux go build
mv dockerize dockerize.linux
GOOS=windows go build
GOOS=darwin go build
mv dockerize dockerize.darwin
case $(uname -s) in
Linux)	ln -s dockerize.linux dockerize
	;;
Darwin) ln dockerize.darwin dockerize
	;;
esac

