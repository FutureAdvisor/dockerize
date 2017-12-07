package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// IsTTY returns true if Stdin is a tty
func IsTTY() bool {
	if fi, err := os.Stdin.Stat(); err != nil || (fi.Mode()&os.ModeCharDevice) != 0 {
		return true
	}
	return false
}

// RegexReplace performs regex replaces with parameter expansion
func RegexReplace(str string, regex string, repl string, count int, lower bool) string {
	out := ""
	lastIndex := 0
	re := regexp.MustCompile(regex)
	for _, v := range re.FindAllSubmatchIndex([]byte(str), count) {
		groups := []string{}
		for i := 0; i < len(v); i += 2 {
			if v[i] > -1 {
				groups = append(groups, str[v[i]:v[i+1]])
			}
		}
		repl = os.Expand(repl, func(s string) string {
			var i int
			fmt.Sscanf(s, "%d", &i)
			return groups[i]

		})
		if lower {
			repl = strings.ToLower(repl)
		}
		out += str[lastIndex:v[0]] + repl
		lastIndex = v[1]
	}
	return out + str[lastIndex:]
}

// ProcessRegexMapList runs a list of regular expressions against a string with parameter expansion
func ProcessRegexMapList(str string, regexList []map[string]string) string {
	for _, remap := range regexList {
		count := 1
		if strings.Contains(remap["flags"], "all") {
			count = -1
		}
		lower := strings.Contains(remap["flags"], "lower")
		str = RegexReplace(str, remap["from"], remap["to"], count, lower)
	}
	return str
}

// ReadTrimmedFile returns the trimmed content of "file"
func ReadTrimmedFile(file string) string {
	content, _ := ioutil.ReadFile(file)
	return strings.TrimRight(string(content), " \t\r\n")
}

// FindFile for finding "file" by searching list of paths in "other"
func FindFile(file string, other ...string) (string, bool) {
	for _, path := range other {
		if path == "." {
			// . is a special case which will scan up from the current directory.
			for path, _ := os.Getwd(); path[len(path)-1] != os.PathSeparator; path = filepath.Dir(path) {
				dir := filepath.Join(path, file)
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					return dir, true
				}
			}
		} else {
			dir := filepath.Join(path, file)
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				return dir, true
			}
		}
	}
	return "", false
}
