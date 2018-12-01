package binpack

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
)

var regFuncName = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// safeFunctionName converts the given name into a name which qualifies as a
// valid func identifier, comparing the name against a known list of funcs to
// prevent conflict based on name translation.
func safeFuncName(name string, knownFuncs map[string]int) string {
	var inBytes, outBytes []byte
	var toUpper bool

	name = strings.ToLower(name)
	inBytes = []byte(name)

	for i := 0; i < len(inBytes); i++ {
		if regFuncName.Match([]byte{inBytes[i]}) {
			toUpper = true
		} else if toUpper {
			outBytes = append(outBytes, []byte(strings.ToUpper(string(inBytes[i])))...)
			toUpper = false
		} else {
			outBytes = append(outBytes, inBytes[i])
		}
	}

	name = string(outBytes)

	// identifier can't start with a digit
	if unicode.IsDigit(rune(name[0])) {
		name = "_" + name
	}

	if num, ok := knownFuncs[name]; ok {
		knownFuncs[name] = num + 1
		name = fmt.Sprintf("%s%d", name, num)
	} else {
		knownFuncs[name] = 2
	}

	return name
}

const lowerHex = "0123456789abcdef"

type StringWriter struct {
	io.Writer
	c int
}

func (w *StringWriter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return
	}

	buf := []byte(`\x00`)
	var b byte

	for n, b = range p {
		buf[2] = lowerHex[b/16]
		buf[3] = lowerHex[b%16]
		w.Writer.Write(buf)
		w.c++
	}

	n++

	return
}

var (
	newline    = []byte{'\n'}
	dataindent = []byte{'\t', '\t'}
	space      = []byte{' '}
)

type ByteWriter struct {
	io.Writer
	c int
}

func (w *ByteWriter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return
	}

	for n = range p {
		if w.c%12 == 0 {
			w.Writer.Write(newline)
			w.Writer.Write(dataindent)
			w.c = 0
		} else {
			w.Writer.Write(space)
		}

		fmt.Fprintf(w.Writer, "0x%02x,", p[n])
		w.c++
	}

	n++

	return
}
