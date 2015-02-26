package main

import "fmt"

type KeyRepository struct {
	//alive bool
	addr string
}

func NewKeyRepository(addr string) (*KeyRepository, error) {
	repo := &KeyRepository{
		addr: addr,
	}

	return repo, nil
}

func (repository KeyRepository) GetShadowEntries(
	logins []string,
) (*ShadowEntries, error) {
	if repository.addr == "a" {
		return nil, fmt.Errorf("fail with A server")
	}

	entry := &ShadowEntry{
		login: logins[0],
		hash:  "$1$blah$blah",
	}

	return &ShadowEntries{entry}, nil
}
