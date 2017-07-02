// +build darwin freebsd dragonfly openbsd netbsd

package main

import "syscall"

const (
	getTermios = syscall.TIOCGETA
	setTermios = syscall.TIOCSETA
)
