package main

import (
  "syscall"
  "unsafe"
  "strings"
  "log"
  "os"
  "os/exec"
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

// Trampoline used to be sufficient until I needed environment handling

func main() {
  env := os.Environ()
  args := []string{}
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
