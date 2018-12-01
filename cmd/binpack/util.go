package main

import (
	"fmt"
	"runtime"
	"strings"
)

const (
	AppName         = "binpack"
	AppVersionMajor = 4
	AppVersionMinor = 0
)

// AppVersionRev is the revision part of the program version.
//
// This will be set automatically at build time like so:
//
//     go build -ldflags "-X main.AppVersionRev `date -u +%s`"
var AppVersionRev string

func Version() string {
	if len(AppVersionRev) == 0 {
		AppVersionRev = "0"
	}

	return fmt.Sprintf(
		"%s %d.%d.%s (Go runtime %s).\nCopyright (c) 2010-2013, Jim Teeuwen.",
		AppName, AppVersionMajor, AppVersionMinor, AppVersionRev, runtime.Version(),
	)
}

// AppendSliceValue implements the flag.Value interface and allows multiple
// calls to the same variable to append a list.
//
// borrowed from https://github.com/hashicorp/serf/blob/master/command/agent/flag_slice_value.go
type AppendSliceValue []string

func (s *AppendSliceValue) String() string {
	return strings.Join(*s, ",")
}

func (s *AppendSliceValue) Set(value string) error {
	if *s == nil {
		*s = make([]string, 0, 1)
	}

	*s = append(*s, value)
	return nil
}
