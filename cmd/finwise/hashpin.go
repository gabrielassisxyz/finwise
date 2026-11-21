package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/api"
)

// maxPINInputBytes keeps a stray pipe from being read to the end; a PIN line is
// six characters.
const maxPINInputBytes = 64

// runHashPIN reads one PIN from standard input and prints its stored form. The
// PIN is never taken from an argument, which would sit in the process list and
// the shell history.
func runHashPIN(env environment) int {
	line, err := bufio.NewReader(io.LimitReader(env.stdin, maxPINInputBytes)).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintln(env.stderr, "finwise: could not read the PIN from standard input")
		return exitFailure
	}
	pin := strings.TrimRight(line, "\r\n")
	if !api.ValidPIN(pin) {
		fmt.Fprintln(env.stderr, "finwise: a PIN is exactly six digits")
		return exitUsage
	}
	stored, err := api.NewPINHash(pin)
	if err != nil {
		fmt.Fprintln(env.stderr, "finwise: could not hash the PIN")
		return exitFailure
	}
	fmt.Fprintln(env.stdout, stored)
	return 0
}
