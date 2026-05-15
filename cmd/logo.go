package cmd

import (
	"fmt"
	"io"
)

const logoPlain = `   ▀▀  █    █    █▀▀█ █  █
  █▀▀█ █    █    █  █ ▀▄▄█
  ▀  ▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀  ▄▄▀`

const logoTTY = "\033[1m   ▀▀  █    █    █▀▀█ █  █\033[0m\n" +
	"\033[1m  █▀▀█ \033[2m█    █    █  █ ▀▄▄█\033[0m\n" +
	"\033[2m  ▀  ▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀  ▄▄▀\033[0m"

func PrintBanner(w io.Writer, tty bool) {
	if tty {
		fmt.Fprintln(w, logoTTY)
	} else {
		fmt.Fprintln(w, logoPlain)
	}
}
