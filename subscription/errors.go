package subscription

import (
	"errors"
	"os"
	"strings"
)

// ShouldTreatAsEmptyResult returns true when source errors should be interpreted
// as "no active proxies" rather than a fatal runtime condition.
func ShouldTreatAsEmptyResult(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no valid proxy configurations found") {
		return true
	}
	if strings.Contains(msg, "failed to read folder") && strings.Contains(msg, "no such file or directory") {
		return true
	}
	if strings.Contains(msg, "error reading file") && strings.Contains(msg, "no such file or directory") {
		return true
	}
	return false
}
