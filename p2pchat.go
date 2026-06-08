package main

import (
	"github.com/chzyer/readline"
)

func newReadline() (*readline.Instance, error) {
	return readline.NewEx(&readline.Config{
		Prompt: "\033[36m>\033[0m ",
	})
}