package sigs

import (
	_ "embed"
)

//go:generate go run gen.go

// nodeKeyring is the node signing keyring.
//
//go:embed nodejs.pub
var NodeJsPub []byte

// yarnKeyring is the yarn siginng keyring.
//
//go:embed yarn.pub
var YarnPub []byte
