package shell

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (r *Runner) parseTimeout(arguments map[string]string) (time.Duration, error) {
	raw := strings.TrimSpace(arguments[argumentTimeoutSeconds])
	if raw == "" {
		return r.defaultTimeout, nil
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, errors.New("timeout_seconds must be a positive integer")
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout > r.maxTimeout {
		return 0, fmt.Errorf("timeout_seconds exceeds maximum %d", int(r.maxTimeout/time.Second))
	}

	return timeout, nil
}
