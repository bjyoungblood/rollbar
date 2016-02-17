package rollbar

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
)

var (
	knownFilePathPatterns = []string{
		runtime.GOROOT() + "/",
		"github.com/",
		"code.google.com/",
		"bitbucket.org/",
		"launchpad.net/",
	}
)

func init() {
	gopath := os.Getenv("GOPATH")
	if gopath != "" {
		knownFilePathPatterns = append(knownFilePathPatterns, gopath)
	}
}

// Frame is a single line of executed code in a Stack.
type Frame struct {
	Filename string `json:"filename"`
	Method   string `json:"method"`
	Line     int    `json:"lineno"`
	Code     string `json:"code,omitempty"`
}

// NewFrame creates a new Frame with the filename shortened in the same way as it
// would be when using BuildStack
func NewFrame(file, method string, line int) Frame {
	code, _ := sourceLine(file, line)
	return Frame{shortenFilePath(file), method, line, code}
}

// Stack represents a stacktrace as a slice of Frames.
type Stack []Frame

// BuildStack builds a full stacktrace for the current execution location.
func BuildStack(skip int) Stack {
	stack := make(Stack, 0)

	for i := skip; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}

		code, _ := sourceLine(file, line)
		file = shortenFilePath(file)
		stack = append(stack, Frame{file, functionName(pc), line, code})
	}

	return stack
}

// Fingerprint builds a string that uniquely identifies a Rollbar item using
// the full stacktrace. The fingerprint is used to ensure (to a reasonable
// degree) that items are coalesced by Rollbar in a smart way.
func (s Stack) Fingerprint() string {
	hash := crc32.NewIEEE()
	for _, frame := range s {
		fmt.Fprintf(hash, "%s%s%d", frame.Filename, frame.Method, frame.Line)
	}
	return fmt.Sprintf("%x", hash.Sum32())
}

// Remove un-needed information from the source file path. This makes them
// shorter in Rollbar UI as well as making them the same, regardless of the
// machine the code was compiled on.
//
// Examples:
//   /usr/local/go/src/pkg/runtime/proc.c -> pkg/runtime/proc.c
//   /home/foo/go/src/github.com/rollbar/rollbar.go -> github.com/rollbar/rollbar.go
func shortenFilePath(s string) string {
	idx := strings.Index(s, "/src/pkg/")
	if idx != -1 {
		return s[idx+5:]
	}
	for _, pattern := range knownFilePathPatterns {
		idx = strings.Index(s, pattern)
		if idx != -1 {
			return s[idx:]
		}
	}
	return s
}

func functionName(pc uintptr) string {
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "???"
	}
	name := fn.Name()
	end := strings.LastIndex(name, string(os.PathSeparator))
	return name[end+1 : len(name)]
}

func sourceLine(file string, lineNumber int) (string, error) {
	data, err := ioutil.ReadFile(file)

	if err != nil {
		return "", err
	}

	lines := bytes.Split(data, []byte{'\n'})
	if lineNumber <= 0 || lineNumber >= len(lines) {
		return "???", nil
	}
	// -1 because line-numbers are 1 based, but our array is 0 based
	return string(bytes.Trim(lines[lineNumber-1], " \t")), nil
}
