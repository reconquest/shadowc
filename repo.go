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

func (repository KeyRepository) GetShadows(
	logins []string,
) (*Shadows, error) {
	if repository.addr == "a" {
		return nil, fmt.Errorf("fail with A server")
	}

	shadow := &Shadow{
		Login: logins[0],
		Hash:  "$1$blah$blah",
	}

	shadows := new(Shadows)
	*shadows = append(*shadows, shadow)

	return shadows, nil
}
