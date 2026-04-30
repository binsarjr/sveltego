package server

import (
	"errors"
	"fmt"
	"strings"
)

const (
	headPlaceholder = "%sveltego.head%"
	bodyPlaceholder = "%sveltego.body%"
)

// errShellEmpty signals an empty shell template.
var errShellEmpty = errors.New("server: shell template is empty")

// parseShell splits an app.html template on %sveltego.head% and
// %sveltego.body%, returning the prefix, middle, and suffix slices.
// Each placeholder must appear exactly once and head must precede body.
func parseShell(src string) (head, mid, tail string, err error) {
	if src == "" {
		return "", "", "", errShellEmpty
	}
	headIdx := strings.Index(src, headPlaceholder)
	if headIdx < 0 {
		return "", "", "", fmt.Errorf("server: shell missing %s placeholder", headPlaceholder)
	}
	if strings.Contains(src[headIdx+len(headPlaceholder):], headPlaceholder) {
		return "", "", "", fmt.Errorf("server: shell has duplicate %s placeholder", headPlaceholder)
	}
	bodyIdx := strings.Index(src, bodyPlaceholder)
	if bodyIdx < 0 {
		return "", "", "", fmt.Errorf("server: shell missing %s placeholder", bodyPlaceholder)
	}
	if strings.Contains(src[bodyIdx+len(bodyPlaceholder):], bodyPlaceholder) {
		return "", "", "", fmt.Errorf("server: shell has duplicate %s placeholder", bodyPlaceholder)
	}
	if bodyIdx < headIdx {
		return "", "", "", fmt.Errorf("server: shell has %s before %s", bodyPlaceholder, headPlaceholder)
	}
	head = src[:headIdx]
	mid = src[headIdx+len(headPlaceholder) : bodyIdx]
	tail = src[bodyIdx+len(bodyPlaceholder):]
	return head, mid, tail, nil
}
