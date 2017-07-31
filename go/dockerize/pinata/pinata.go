package pinata

import (
	"dockerize/docker"
	"dockerize/utils"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/crypto/ssh/agent"
)

// PinataVersion is the version we will be building the container with.
const PinataVersion = "2.0.0"

var homeDir string

func copySSHFiles() {}

// AgentHasKeys returns true if there is an agent and it has keys
func AgentHasKeys() bool {
	runningAgent := false
	if sshAgentSock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		defer sshAgentSock.Close()
		sshAgent := agent.NewClient(sshAgentSock)
		if keys, error := sshAgent.List(); error == nil {
			if len(keys) > 0 {
				runningAgent = true
			}
		}
	}
	return runningAgent
}

// ForwardSSH starts up a pinata-sshd container and forwards our agent to it.
func ForwardSSH(home string) error {
	homeDir = home

	// copy ssh files to that location if necessary
	copySSHFiles()
	privateKey := home + "/.ssh/id_rsa.pub"
	pinataHome := home + "/.pinata-sshd"
	// cleanup local state location ($home/.pinata-sshd)
	// define our container and local state location
	pinataSSH := docker.Container{
		Image: "pinata-sshd",
		Name:  "pinata-sshd",
		Tag:   PinataVersion,
		Volumes: []string{
			privateKey + ":/root/.ssh/authorized_keys",
			"/tmp:/tmp:delegated",
			pinataHome + ":/share:delegated",
		},
		Port:  "2244:22",
		Flags: "-d --init",
	}
	// run container - local state to /share and /tmp:/tmp
	instance, err := pinataSSH.Run()
	fmt.Printf("pinata id: %s\n", instance)
	// get the host ip for the container with docker inspect
	hostIP := pinataSSH.HostIP
	// ssh-keyscan and add to known hosts (or should we just ignore hosts file?)
	// start ssh to the container (forked to background)
	knownHostContent, _ := exec.Command("ssh-keyscan", "-p", "2244", hostIP).Output()
	ioutil.WriteFile(pinataHome+"known_hosts", []byte(knownHostContent), 0644)
	cmd := exec.Command("ssh", "-f", "-o", "UserKnownHostsFile="+pinataHome+"/known_hosts",
		"-A", "-p", "2244", "root@"+hostIP, "/root/ssh-find-agent.sh")
	cmd.Start()
	// return OK!
	return err
}

// Volume returns the docker volume mount
func Volume() string {
	pinataHome := homeDir + "/.pinata-sshd"
	pinataSock := utils.ReadTrimmedFile(filepath.Join(pinataHome, "agent_socket_path"))
	return pinataSock + ":/tmp/ssh-agent.sock"
}

// Environment returns the environment variable for the container
func Environment() string {
	return "SSH_AUTH_SOCK=/tmp/ssh-agent.sock"
}
