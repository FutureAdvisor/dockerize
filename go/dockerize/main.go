package main

import (
	"dockerize/docker"
	"dockerize/impl"
	"dockerize/pinata"
	"dockerize/utils"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
	runner = impl.RealRunner{}
)

func osdetect() string {
	theos := impl.GOOS
	if theos == "linux" {
		if text, err := runner.ReadFile("/proc/sys/kernel/osrelease"); err == nil && strings.Contains(string(text), "Microsoft") {
			theos = "linux/windows"
		}
	}
	return theos
}

/*
	hashify saves me from having to write search functions
	maybe they exist natively.
*/
func hashify(list []string) map[string]bool {
	ret := make(map[string]bool)
	for _, s := range list {
		ret[s] = true
	}
	return ret
}

/*
	exclude items in a map from a list
*/

func exclude(stringList []string, exclmap map[string]bool) []string {
	ret := make([]string, 0, len(stringList))
	for _, s := range stringList {
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

/*
	I don't know how many of the Windows variables are set by default.
	This rids us of most of the trash
*/

var (
	/* nixExclude contains the variables we know we don't want to pass into the container for unix */
	nixExclude = []string{"SHLVL", "SHELL", "HOSTTYPE", "_", "PATH", "DOCKER_HOST", "SSH_AUTH_SOCK",
		"SSH_AGENT_PID", "LS_COLORS", "PWD"}
	/* winExclude contains the variables we know we don't want to pass into the container from Windows */
	winExclude = []string{"", "ALLUSERSPROFILE", "APPDATA", "asl.log", "CommonProgramFiles",
		"CommonProgramFiles(x86)", "CommonProgramW6432", "COMPUTERNAME", "ComSpec",
		"HOMEDRIVE", "HOMEPATH", "LOCALAPPDATA", "LOGONSERVER",
		"NUMBER_OF_PROCESSORS", "OneDrive", "OS", "Path", "PATHEXT", "PROCESSOR_ARCHITECTURE",
		"PROCESSOR_IDENTIFIER", "PROCESSOR_LEVEL", "PROCESSOR_REVISION", "ProgramData",
		"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "PROMPT", "PSModulePath",
		"PUBLIC", "SESSIONNAME", "SystemDrive", "SystemRoot", "TEMP", "TMP", "USERDOMAIN",
		"USERDOMAIN_ROAMINGPROFILE", "USERNAME", "USERPROFILE", "VS110COMNTOOLS",
		"VS120COMNTOOLS", "VS140COMNTOOLS", "windir"}
	port     TranslationOptions
	config   Config
	commands CommandMap
	theos    = osdetect()
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

// Config is a structure used for JSON Marshalling
// Currently the only top level member is a map:
// containers.
type Config struct {
	Containers  ContainerMap `json:"containers"`
	Portability OSMap        `json:"portability"`
}

// ContainerOptions is a structure to store the
// data specific to a particular container
type ContainerOptions struct {
	Commands []string `json:"commands"`
	Volumes  []string `json:"volumes"`
}

// TranslationOptions is a structure to store translations for OS specifics
type TranslationOptions struct {
	Home             string              `json:"home"`
	PathRegex        []map[string]string `json:"path_regex"`
	ExecutableSuffix string              `json:"executable_suffix"`
}

// ContainerMap is a simple map indexed by container name
type ContainerMap map[string]ContainerOptions

// OSMap is a simple map indexed by OS Match
type OSMap map[string]TranslationOptions

// CommandMap is a simple map used to reverse index the containers by command
type CommandMap map[string]string

/*
	readConfig reads a configuration file from "name" and outputs
	a CommandMap
*/
func readConfig(name string) {
	commands = CommandMap{}
	config = Config{}
	var configBytes []byte
	if bytes, err := ioutil.ReadFile(name); err == nil {
		configBytes = bytes
	} else {
		configBytes = make([]byte, len(dockerizeJSON))
		copy(configBytes, dockerizeJSON)
	}
	// I like comments, but the JSON parser doesn't
	re := regexp.MustCompile("(?m)^\\s*//.*$")
	configBytes = re.ReplaceAllLiteral(configBytes, []byte{})
	if err := json.Unmarshal(configBytes, &config); err != nil {
		fmt.Printf("unable to read configuration: %s\n", err)
		os.Exit(1)
	}
	for containerName, options := range config.Containers {
		for _, command := range options.Commands {
			commands[command] = containerName
		}
	}
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

func installSymlinks() {
	self, _ := os.Executable()
	dir := filepath.Dir(self)
	for cmd := range commands {
		if osdetect() == "windows" {
			cmd += ".exe"
		}
		cmd = filepath.Join(dir, cmd)
		os.Remove(cmd)
		os.Link(self, cmd)
	}

}

const dockerizeUsageString = "dockerize init - setup docker for using dockerize\n" +
	"dockerize install <path> - install symlinks to known programs to path\n" +
	"dockerize help - this text\n"

func dockerize() {
	if len(os.Args) < 2 || os.Args[1] == "help" {
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

// ConvertPath is a helper function for applying regular expressions
func ConvertPath(path string) string {
	return utils.ProcessRegexMapList(path, port.PathRegex)
}

func processPortability() {
	port = config.Portability["default"]
	over := config.Portability[theos]
	if over.Home != "" {
		port.Home = over.Home
	}
	if over.PathRegex != nil {
		port.PathRegex = over.PathRegex
	}
	if over.ExecutableSuffix != "" {
		port.ExecutableSuffix = over.ExecutableSuffix
	}
	port.Home, _ = os.LookupEnv(port.Home)
}

func appendTTYFlags(args []string) []string {
	if utils.IsTTY() {
		args = append(args, "-it")
	}
	return args
}

func doWith() {}
func runCommand(command string) {
	var (
		remove []string
	)
	absSelf := "/tmp/dockerize"
	pwd, _ := os.Getwd()
	pwd = ConvertPath(pwd)
	fmt.Printf("pwd: %s, cmd: %s, image: %s\n", pwd, command, commands[command])

	if theos == "windows" {
		remove = winExclude
	} else {
		remove = nixExclude
	}
	env := exclude(os.Environ(), hashify(remove))
	containername := commands[command]
	/*
		cleanContainername := strings.Replace(containername, "/", "__", -1)
		versionfile := fmt.Sprintf(".%s-version", containername)
		prefix := containername + "-"
		if versionfile, found := utils.FindFile(versionfile, ".", port.Home); found {
			containerversion = utils.ReadTrimmedFile(versionfile)
		}
		if len(containerversion) > len(prefix) && containerversion[0:len(prefix)] == prefix {
			containerversion = containerversion[len(prefix):]
		}
		if containerversion == "" {
			containerversion = "latest"
		}
		os.Setenv("container_version", containerversion)
	*/
	container := docker.Container{Image: containername}
	if running, _ := container.IsRunning(); !running {
		pinata.ForwardSSH(port.Home)
		containerVolumes := []string{
			ConvertPath(port.Home) + ":" + ConvertPath(port.Home) + ":cached",
			ConvertPath(port.Home) + "/.ssh/known_hosts:/etc/ssh/ssh_known_hosts",
			absSelf + ":/bin/execwdve:ro"}
		for _, volume := range config.Containers[containername].Volumes {
			expanded := os.ExpandEnv(volume)
			containerVolumes = append(containerVolumes, expanded)
		}
		container.Volumes = containerVolumes
		container.Cmd = "cat"
		container.Run()
	}
	args := []string{"exec"}
	args = appendTTYFlags(args)
	args = append(args, container.ID, "/bin/execwdve")
	args = append(args, env...)
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
	if err := docker.Connect(); err != nil {
		fmt.Printf("Unable to connect to docker socket: %s\n", err)
		os.Exit(1)
	}
	basename, _ := os.Executable()
	if configPath, _ := utils.FindFile("dockerize.json", filepath.Dir(basename), "."); true {
		readConfig(configPath)
		processPortability()
	}
	basename = filepath.Base(os.Args[0])
	basename = strings.TrimSuffix(basename, port.ExecutableSuffix)
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
