#!/bin/bash
echo "building execwdve"
home=$HOME
test -n "$USERPROFILE" && home=$USERPROFILE
CTR=$(docker run -d -v ${PWD}:/go/src golang:latest go install execwdve)
# wait for build
while test -n "$(docker ps -qf id=${CTR})"; do sleep 1; docker ps -qf id=${CTR}; done
docker logs ${CTR}
mkdir -p ${home}/bin
docker cp ${CTR}:/go/bin/execwdve ${home}/bin
docker rm ${CTR}
