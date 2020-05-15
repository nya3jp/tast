package dbg

import "fmt"

func wrap(x int) func(string) string {
	return func(s string) string {
		return fmt.Sprintf("\033[%dm%s\033[0m", x, s)
	}
}

var Black = wrap(30)
var Red = wrap(31)
var Green = wrap(32)
var Brown = wrap(33)
var Blue = wrap(34)
var Magenta = wrap(35)
var Cyan = wrap(36)
var Gray = wrap(37)
