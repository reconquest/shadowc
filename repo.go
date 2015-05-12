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
	users []string, pool string,
) (*Shadows, error) {

	shadows := new(Shadows)
	for _, user := range users {
		hash, err := repository.getHash(user, pool)
		if err != nil {
			return nil, err
		}

		proofHash, err := repository.getHash(user, pool)
		if err != nil {
			return nil, err
		}

		if hash == proofHash {
			log.Printf(
				"Warning, hash for user '%s' was recently requested; "+
					"possible break in attempt.",
				user,
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

func (repository KeyRepository) getHash(
	user string, pool string,
) (string, error) {
	response, err := repository.resource.Get(
		repository.addr + "/t/" + user + "/" + pool,
	)
	if err != nil {
		return "", err
	}

	if response.StatusCode != 200 {
		if response.StatusCode == 404 {
			return "", fmt.Errorf(
				"hash table for user '%s' not found", user,
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
