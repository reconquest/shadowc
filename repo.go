package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type KeyRepository struct {
	addr     string
	resource *http.Client
}

func NewKeyRepository(addr string, resource *http.Client) (*KeyRepository, error) {
	addr = strings.TrimRight(addr, "/")
	if strings.HasPrefix(addr, "http://") {
		addr = addr[7:]
	}

	if !strings.HasPrefix(addr, "https://") {
		addr = "https://" + addr
	}

	repository := &KeyRepository{
		addr:     addr,
		resource: resource,
	}

	return repository, nil
}

func (repository KeyRepository) GetShadows(
	pool string, users []string,
) (*Shadows, error) {
	var token string

	shadows := new(Shadows)
	for _, user := range users {
		if pool != "" {
			token = pool + "/" + user
		} else {
			token = user
		}

		hash, err := repository.getHash(token)
		if err != nil {
			return nil, err
		}

		proofHash, err := repository.getHash(token)
		if err != nil {
			return nil, err
		}

		if hash == proofHash {
			log.Printf(
				"Warning, hash for token '%s' was recently requested; "+
					"possible break in attempt.",
				token,
			)
		}

		shadow := &Shadow{
			User: user,
			Hash: hash,
		}

		*shadows = append(*shadows, shadow)
	}

	return shadows, nil
}

func (repository KeyRepository) getHash(token string) (string, error) {

	response, err := repository.resource.Get(
		repository.addr + "/t/" + token,
	)
	if err != nil {
		return "", err
	}

	if response.StatusCode != 200 {
		if response.StatusCode == 404 {
			return "", fmt.Errorf(
				"hash table for token '%s' not found", token,
			)
		}

		return "", fmt.Errorf("error HTTP status: %s", response.Status)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	return strings.TrimRight(string(body), "\n"), nil
}
