package binpack

import (
	"testing"
)

func TestSafeFuncName(t *testing.T) {
	var knownFuncs = make(map[string]int)
	name1 := safeFuncName("foo/bar", knownFuncs)
	name2 := safeFuncName("foo_bar", knownFuncs)
	if name1 == name2 {
		t.Errorf("name collision")
	}
}
