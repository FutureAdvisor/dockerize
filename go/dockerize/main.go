package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

type Termios struct {
	Iflag  uint64
	Oflag  uint64
	Cflag  uint64
	Lflag  uint64
	Cc     [20]byte
	Ispeed uint64
	Ospeed uint64
}

// TcSetAttr restores the terminal connected to the given file descriptor to a
// previous state.
func TcSetAttr(fd uintptr, termios *Termios) error {
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(setTermios), uintptr(unsafe.Pointer(termios))); err != 0 {
		return err
	}
	return nil
}

// TcGetAttr retrieves the current terminal settings and returns it.
func TcGetAttr(fd uintptr) (*Termios, error) {
	var termios = &Termios{}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, getTermios, uintptr(unsafe.Pointer(termios))); err != 0 {
		return nil, err
	}
	return termios, nil
}

func stty_onlcr(fd uintptr) (*Termios, error) {
	old, err := TcGetAttr(fd)
	if err != nil {
		return nil, err
	}

	new := *old
	new.Oflag |= syscall.ONLCR

	if err := TcSetAttr(fd, &new); err != nil {
		return nil, err
	}
	return old, nil
}

/*
	These data structures are for unmarshalling the JSON data
	I load them into a map by command, but the file is much more
	convenient to write by container.
	This might in future include other things per container than
	supported commands.
	It might also include other global options than "containers".
	{"containers":{"golang":{"commands":["go"]}}}
	is current valid syntax.
*/
type Containers struct {
	Container ContainerMap `json:"containers"`
}

type ContainerOptions struct {
	Commands []string `json:"commands"`
}

type ContainerMap map[string]ContainerOptions

/*
	Reads a configuration file from "name" and outputs a map of commands to containers
*/
func readConfig(name string) *map[string]*string {
	containers := []string{}
	commands := map[string]*string{}
	config := Containers{}
	if err := json.Unmarshal([]byte(name), &config); err != nil {
		config_bytes, _ := ioutil.ReadFile(name)
		// if I didn't need the command index, we'd be done by now
		json.Unmarshal(config_bytes, &config)
	}
	for ctr, cmds := range config.Container {
		// fmt.Printf("%d %s\n", len(containers), ctr)
		pos := len(containers)
		containers = append(containers, ctr)
		for _, cmd := range cmds.Commands {
			commands[cmd] = &containers[pos]
			// fmt.Printf("\t%s->%s\n", cmd, *commands[cmd])
		}
	}
	return &commands
}

func homeDir() string {
	if home, present := os.LookupEnv("USERPROFILE"); present {
		return home
	} else {
		home, _ := os.LookupEnv("HOME")
		return home
	}
}

func loadup(file string) string {
	path, _ := os.Getwd()
	for path[len(path)-1] != os.PathSeparator {
		if _, err := os.Stat(path + string(os.PathSeparator) + file); !os.IsNotExist(err) {
			break
		}
		// fmt.Println(path)
		path = filepath.Dir(path)
	}
	if path[len(path)-1] == os.PathSeparator {
		dir := homeDir() + string(os.PathSeparator) + file
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			path = homeDir()
		}
	}
	// fmt.Println(path)
	if path[len(path)-1] != os.PathSeparator {
		content, _ := ioutil.ReadFile(path + string(os.PathSeparator) + file)
		return strings.TrimRight(string(content), " \t\r\n")
	}
	return ""
}

// Trampoline used to be sufficient until I needed environment handling

func main() {
	env := os.Environ()
	args := []string{}
	commands := readConfig(loadup("dockerize.json"))
	fmt.Println(commands)
	version := loadup(".ruby-version")
	fmt.Printf("%s\n", version)
	os.Exit(1)
	workdir := ""
	stage := 0
	for _, arg := range os.Args[1:] {
		if strings.Contains(arg, "-onlcr") {
			fd := uintptr(syscall.Stdout)
			stty_onlcr(fd)
		} else {
			if stage == 0 && strings.Contains(arg, "=") {
				env = append(env, arg)
			} else {
				if stage == 0 {
					workdir = arg
					stage++
				} else {
					args = append(args, arg)
				}
			}
		}
	}
	if len(args) == 0 {
		log.Fatal("Usage: execwdve [<env>=<val>]... <workdir> <cmd> [<args>]...")
	}
	err := syscall.Chdir(workdir)
	if err != nil {
		log.Fatalf("Can't change to working directory '%s'\n%s", workdir, err)
	}
	executable := ""
	if strings.Contains(args[0], "/") {
		executable = args[0]
	} else {
		executable, err = exec.LookPath(args[0])
		if err != nil {
			log.Fatalf("Can't find '%s' in the path\n%s", args[0], err)
		}
	}
	err = syscall.Exec(executable, args, env)
	log.Println(err)
}
