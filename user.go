package main

import "strings"

type user struct {
	name, pool string
}

func (user user) String() string {
	if user.pool == "" {
		return "user " + user.name
	}

	return "user " + user.name + " within pool " + user.pool
}

type users struct {
	names []string
	pool  string
}

func (users users) String() string {
	if users.pool == "" {
		return "users " + strings.Join(users.names, ", ")
	}

	return "users " + strings.Join(users.names, ", ") +
		" within pool " + users.pool
}
