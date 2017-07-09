package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
	"syscall"
)

/*
	Our operations use the host version of docker-engine and what
	constitutes a safe path mean that we need to be able to tell
	the difference between linux and wsl.

	on native linux we should use the user's home directory but
	Windows likes to segregate the user home directory inside
	the Linux filesystem, which only WSL can see, so on Windows
	we need to use the USERPROFILE directory.
*/

// This will generate us a jsonfiles.go with our dockerize_json inside
//go:generate go run scripts/includeconfig.go

var (
	goos = runtime.GOOS
	ReadFile = ioutil.ReadFile
)

func osdetect() string {
	theos := goos
	if theos == "linux" {
		if text, err := ReadFile("/proc/sys/kernel/osrelease"); err == nil && strings.Contains(string(text), "Microsoft") {
			theos = "linux/windows"
		}
	}
	return theos
}

func hashify(list []string) map[string]int {
	ret := make(map[string]int)
	for _, s := range list {
		ret[s] = 1
	}
	return ret
}

func exclude(ss []string, exclmap map[string]int) []string {
	ret := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := exclmap[s[0:strings.IndexRune(s, '=')]]; !ok {
			ret = append(ret, s)
		}
	}
	return ret
}

// This string does double duty to check we installed ourselves OK.
const (
	usageString = "Usage: execwdve [<env>=<val>]... <workdir> <cmd> [<args>]..."
)

var (
	nixExclude = []string{"SHLVL", "SHELL", "HOSTTYPE", "_", "PATH", "DOCKER_HOST", "SSH_AUTH_SOCK",
		"SSH_AGENT_PID", "LS_COLORS", "PWD"}
	winExclude = []string{"", "ALLUSERSPROFILE", "APPDATA", "asl.log", "CommonProgramFiles",
		"CommonProgramFiles(x86)", "CommonProgramW6432", "COMPUTERNAME", "ComSpec",
		"HOMEDRIVE", "HOMEPATH", "LOCALAPPDATA", "LOGONSERVER",
		"NUMBER_OF_PROCESSORS", "OneDrive", "OS", "Path", "PATHEXT", "PROCESSOR_ARCHITECTURE",
		"PROCESSOR_IDENTIFIER", "PROCESSOR_LEVEL", "PROCESSOR_REVISION", "ProgramData",
		"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "PROMPT", "PSModulePath",
		"PUBLIC", "SESSIONNAME", "SystemDrive", "SystemRoot", "TEMP", "TMP", "USERDOMAIN",
		"USERDOMAIN_ROAMINGPROFILE", "USERNAME", "USERPROFILE", "VS110COMNTOOLS",
		"VS120COMNTOOLS", "VS140COMNTOOLS", "windir"}
)

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
	Volumes []string `json:"volumes"`
}

type ContainerMap map[string]ContainerOptions

/*
	Reads a configuration file from "name" and outputs a map of commands to containers
*/
func readConfig(name string) (Containers, map[string]*string) {
	containers := []string{}
	commands := map[string]*string{}
	config := Containers{}
	var config_bytes []byte
	if bytes, err := ioutil.ReadFile(name); err == nil {
		config_bytes = bytes
	} else {
		config_bytes = make([]byte,len(dockerize_json))
		copy(config_bytes, dockerize_json)
	}
	// I like comments, but the JSON parser doesn't
	re := regexp.MustCompile("(?m)//.*$")
	config_bytes = re.ReplaceAllLiteral(config_bytes, []byte{})
	if err := json.Unmarshal(config_bytes, &config); err != nil {
		fmt.Printf("unable to read configuration: %s\n", err)
		os.Exit(1)
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
	return config, commands
}

func homeDir() string {
	switch osdetect() {
	case "windows":
		home, _ := os.LookupEnv("USERPROFILE")
		home = strings.Replace(home, "\\", "/", -1)
		home = strings.Replace(home, ":", "",1)
		r, n := utf8.DecodeRuneInString(home)
		home = string(unicode.ToLower(r)) + home[n:]
		home = "/" + home
		return home
	case "linux/windows":
		home, _ := os.LookupEnv("USERPROFILE")
		return home
	default:
		home, _ := os.LookupEnv("HOME")
		return home
	}
}

func search(file string) (string, bool) {
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
		return path + string(os.PathSeparator) + file, true
	}
	return "", false
}

func loadup(file string) string {
	if path, found := search(file); found {
		content, _ := ioutil.ReadFile(path)
		return strings.TrimRight(string(content), " \t\r\n")
	}
	return ""
}

func execwdve() {
	env := os.Environ()
	args := []string{}
	workdir := ""
	stage := 0
	for _, arg := range os.Args[1:] {
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
	if len(args) == 0 {
		fmt.Println(usageString)
		os.Exit(0)
	}
	err := syscall.Chdir(workdir)
	if err != nil {
		fmt.Printf("Can't change to working directory '%s'\n%s", workdir, err)
		os.Exit(1)
	}
	executable := ""
	if strings.Contains(args[0], "/") {
		executable = args[0]
	} else {
		executable, err = exec.LookPath(args[0])
		if err != nil {
			fmt.Printf("Can't find '%s' in the path\n%s", args[0], err)
			os.Exit(2)
		}
	}
	err = syscall.Exec(executable, args, env)
	fmt.Printf("Can't exec: %s", err.Error())
}

func head(text string) string {
	outputLines := strings.SplitN(text, "\n", 1)
	output := ""
	if outputLines != nil && len(outputLines) > 0 {
		output = strings.TrimRight(outputLines[0], "\n\r")
	}
	return output
}

func copySelfToTemp() {
	absSelf, _ := filepath.Abs(os.Args[0])
	if osdetect() != "linux" {
		// currently I know about .exe and no extension
		// .linux will be this binary built for linux
		absSelf = strings.TrimSuffix(absSelf, ".exe") + ".linux"
	}
	cmdOutput, _ := exec.Command("docker", "run", "-i", "--rm", "-v", absSelf+":/bin/dockerize:ro",
		"-v", "/tmp:/share", "alpine", "cp", "/bin/dockerize", "/share/").CombinedOutput()
	fmt.Print(string(cmdOutput))
	cmdOutput, _ = exec.Command("docker", "run", "-i", "--rm", "-v", "/tmp/dockerize:/bin/execwdve:ro",
		"alpine", "execwdve").CombinedOutput()
	output := string(cmdOutput)
	if len(output) > len(usageString) && usageString == output[0:len(usageString)] {
		fmt.Println("successfully installed.")
		os.Exit(0)
	} else {
		fmt.Println("installation failed.")
		os.Exit(1)
	}
}

func installSymlinks() {}

const dockerizeUsageString = "dockerize init - setup docker for using dockerize\n" +
	"dockerize install <path> - install symlinks to known programs to path"

func dockerize() {
	if len(os.Args) < 2 {
		fmt.Println(dockerizeUsageString)
		os.Exit(0)
	}
	switch os.Args[1] {
	case "install":
		installSymlinks()
	case "init":
		copySelfToTemp()
	}
}

func doWith() {}
func runCommand(command string) {
	var remove []string
	absSelf := "/tmp/dockerize"
	theos := osdetect()
	pwd, _ := os.Getwd()
	if theos == "windows" {
		remove = winExclude
		pwd = strings.Replace(pwd, "\\", "/", -1)
		pwd = strings.Replace(pwd, ":", "",1)
		r, n := utf8.DecodeRuneInString(pwd)
		pwd = string(unicode.ToLower(r)) + pwd[n:]
		pwd = "/" + pwd
	} else {
		remove = nixExclude
		if strings.Contains(theos,"windows") {
			pwd = strings.TrimPrefix(pwd,"/mnt")
		}
	}
	env := exclude(os.Environ(), hashify(remove))
	var (
		config Containers
		commands map[string]*string
	)
	path, _ := search("dockerize.json")
	config, commands = readConfig(path)
	containername := *commands[command]
	cleanContainername := strings.Replace(containername, "/", "__", -1)
	versionfile := fmt.Sprintf(".%s-version", containername)
	prefix := containername + "-"
	containerversion := loadup(versionfile)
	if len(containerversion) > len(prefix) && containerversion[0:len(prefix)] == prefix {
		containerversion = containerversion[len(prefix):]
	}
	if containerversion == "" {
		containerversion = "latest"
	}
	os.Setenv("container_version", containerversion)
	instanceName := cleanContainername + "_" + containerversion
	cmdOutput, _ := exec.Command("docker", "ps", "-qf", "name="+instanceName).Output()
	var instance string
	if instance = head(string(cmdOutput)); instance == "" {
		containerVolumes := []string{"-v", homeDir() + ":" + homeDir(),
			"-v", "~/.ssh/known_hosts:/etc/ssh/ssh_known_hosts",
			"-v", absSelf + ":/bin/execwdve:ro"}
		for _, volume := range config.Container[containername].Volumes {
			expanded := os.ExpandEnv(volume)
			containerVolumes = append(containerVolumes, "-v", expanded)
		}
		fmt.Printf("mounts: %s\n", strings.Join(containerVolumes, " "))
	}
	args := append([]string{"exec","-it", instance, "/bin/execwdve"}, env...)
	args = append(args, pwd, command)
	args = append(args, os.Args[1:]...)
	cmd := exec.Command("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

}

/*
	Command processing is fun
	dockerize install <location> (all known symlinks) to $0
	dockerize init (copy itself into docker host /tmp)
	dockerize write config
	dockerize upgrade (I think that can go away)
	with container command (very useful)
	command ...
*/

func main() {
	basename := filepath.Base(os.Args[0])
	if osdetect() == "windows" {
		basename = strings.TrimSuffix(basename, ".exe")
	}
	os.Setenv("HOME", homeDir())
	switch basename {
	case "execwdve":
		execwdve()
	case "dockerize":
		dockerize()
	case "with":
		doWith()
	default:
		runCommand(basename)
	}
	os.Exit(0)
}
