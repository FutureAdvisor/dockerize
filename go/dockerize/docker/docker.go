// Package docker for running stuff in containers
package docker

import (
	"context"
	"dockerize/utils"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
)

// Client is a docker client instance
var Client *client.Client

// Container is a structure for storing container information
type Container struct {
	ID          string
	Name        string
	Image       string
	Tag         string
	Port        string
	HostIP      string
	Flags       string
	Cmd         string
	Volumes     []string
	Environment []string
}

// Connect the docker client or return an error
func Connect() (err error) {
	Client, err = client.NewEnvClient()
	return err
}

// StandardName returns the standard name for the container
func (c Container) getName() (result string) {
	if c.Name != "" {
		result = c.Name
		return
	}
	result = strings.Replace(c.Image, "/", "__", -1)
	result += "_" + c.getTag()
	return
}

// getTag returns a sane default for the tag or the specific one that has been set
func (c Container) getTag() (containerVersion string) {
	if c.Tag != "" {
		containerVersion = c.Tag
	} else {
		versionFile := fmt.Sprintf(".%s-version", c.Image)
		prefix := c.Image + "-"
		containerVersion = utils.ReadTrimmedFile(versionFile)
		strings.TrimPrefix(containerVersion, prefix)
	}
	if containerVersion == "" {
		containerVersion = "latest"
	}
	c.Tag = containerVersion
	// TODO(faye): this isn't the right place to set this, but I don't want to forget it.
	os.Setenv("container_version", containerVersion)
	return
}

// IsRunning determines if a given container is running by using docker ps
// By side effect, it may update the container id
func (c Container) IsRunning() (bool, error) {

	instanceName := c.getName()
	fmt.Printf("%s\n", instanceName)
	fil := filters.NewArgs()
	fil.Add("name", instanceName)
	if ctrInstances, err := Client.ContainerList(context.Background(), types.ContainerListOptions{Filters: fil}); err == nil {
		if len(ctrInstances) > 0 {
			docker := ctrInstances[0]
			c.ID = docker.ID[0:12]
			return true, nil
		}
	} else {
		return false, err
	}
	return false, nil
}

// Event structure defines the JSON messages sent during a pull
type Event struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Error          string `json:"error"`
	Progress       string `json:"progress"`
	ProgressDetail struct {
		Current int `json:"current"`
		Total   int `json:"total"`
	} `json:"progressDetail"`
}

// Run a container with some mounts
func (c Container) Run() (string, error) {
	running, err := c.IsRunning()
	if err != nil {
		return "", err
	}

	if running {
		return c.ID, nil
	}
	fullImage := c.Image + ":" + c.getTag()
	if out, err := Client.ImagePull(context.Background(), fullImage, types.ImagePullOptions{}); err == nil {
		dec := json.NewDecoder(out)
		for {
			var m Event
			if err := dec.Decode(&m); err == io.EOF {
				break
			} else if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%s:%s\n", m.ID, m.Status)
		}
	} else {
		fmt.Printf("error while pulling image:%s\n", err)
	}

	config := container.Config{
		Image: fullImage,
		Tty:   true,
		Cmd:   strslice.StrSlice{c.Cmd}}
	hostConfig := container.HostConfig{Binds: c.Volumes}
	if resp, err := Client.ContainerCreate(context.Background(), &config, &hostConfig, nil, c.getName()); err == nil {
		c.ID = resp.ID
	} else {
		fmt.Printf("%s\n", err)
		return "", err
	}
	//containerVolumes := "$(pinata_mount_$os) $container_volumes"
	//cleanup := "$(docker rm ${clean_container}_${container_version} 2>/dev/null)"
	fmt.Printf("Can't actually start an instance yet.\n")
	os.Exit(1)
	return c.ID, nil
}

// Exec a program in a container
func (c Container) Exec() {}
