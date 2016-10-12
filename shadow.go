package main

import (
	"strings"
)

type (
	Shadow struct {
		Username string
		Hash     string
	}

	Shadows []*Shadow
)

func (shadows *Shadows) String() string {
	str := []string{}
	for _, shadow := range *shadows {
		str = append(str, shadow.String())
	}

	return strings.Join(str, "\n")
}

func (shadow *Shadow) String() string {
	str := make([]string, 9)
	str[0] = shadow.Username
	str[1] = shadow.Hash

	return strings.Join(str, ":")
}
