package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type KeyRepository struct {
	addr     string
	resource *http.Client
}

func NewKeyRepository(addr string, resource *http.Client) (*KeyRepository, error) {
	addr = strings.TrimRight(repository.addr, "/")
	if strings.HasPrefix(addr, "http://") {
		addr = "https://" + addr[7:0]
	}

	repository := &KeyRepository{
		addr:     addr,
		resource: resource,
	}

	return repository, nil
}

func (repository KeyRepository) GetShadows(
	users []string,
) (*Shadows, error) {

	shadows := new(Shadows)
	for _, user := range users {
		response, err := repository.resource.Get(
			repository.addr + "/t/" + user,
		)

		if err != nil {
			return nil, err
		}

		if response.StatusCode != 200 {
			return nil, fmt.Errorf("error HTTP status: %s", response.Status)
		}

		body, err := ioutil.ReadAll(response.Body)

		if err != nil {
			return nil, err
		}

		defer response.Body.Close()

		shadow := &Shadow{
			User: user,
			Hash: strings.TrimRight(string(body), "\n"),
		}

		*shadows = append(*shadows, shadow)
	}

	return shadows, nil
}
