package params

import "strconv"

func Match(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
