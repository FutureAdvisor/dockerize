#!/bin/bash
fail() {
	echo "$@"
	exit 1
}

should() {
	let testnumber=testnumber+1
	testname=("$@")
	testoutput="$(bash)"
	result=$?
        if [ "${testname[0]}" = "not" ]; then
		test $result -eq 0 && return 1
		return 0
	fi
	return $result
}

it() {
	if "$@"; then
          let testpass=testpass+1
	else
	  echo "it $@: FAILED"
	  echo "$testoutput"
	fi
}

# Are we already running in docker?
grep -q docker /proc/$$/cgroup 2>/dev/null
if [ $? -ne 0 ]; then
	# These tests will run in a standardized golang container
	# which we then remove golang from - go figure.
	docker run -it --rm -v /var/run/docker.sock:/var/run/docker.sock -v /usr/bin/docker:/usr/bin/docker:ro -v ${PWD}:${PWD} -w ${PWD} golang:latest "$0" "$@"
	exit
fi
uname -a
test -S /var/run/docker.sock || fail "You need to pass in docker and the docker socket."
version=$(docker version 2>&1) || fail -e "Docker is not functional in the container.\n$version"
set -o errexit
eval $(ssh-agent) >& /dev/null
# no cheating
rm -rf /usr/local/go
./build.sh >& /dev/null
ssh-keygen -N "" -f ~/.ssh/id_rsa -C "testy@mctest.face" >& /dev/null
ssh-add >& /dev/null
rm -rf bin
mkdir -p bin
mv ~/bin/execwdve bin/
export HOME=${PWD}
export PATH=~/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
# Things should look ok from here
cp dockerize ~/bin
chmod +x ~/bin/dockerize

it should not already have golang installed << EOT
  test -e /usr/local/bin/golang
EOT

it should return successfully when installing go << EOT
  dockerize install golang go
EOT

it should have created golang hardlink << EOT
  test -e bin/golang
EOT

it should have created a symlink to go <<EOT
  test -L bin/go
EOT

it should have the same inode for containers <<EOT
  [ "$(ls -i bin/golang | cut -d' ' -f1)" = "$(ls -i bin/dockerize | cut -d' ' -f1)" ]
EOT

it should be running the correct binary <<EOT
  test "$(which go)" = "$PWD/bin/go"
EOT

it should run go from a container <<EOT
  go version | grep -q go.*amd64
EOT

rm -rf bin
echo
test "$testpass" -gt 0 && echo -n "$testpass/$testnumber tests passed"
test "$testpass" -eq "$testnumber" && echo -n "!"
result=$?
echo
exit $result
