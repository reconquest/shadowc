package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type ShadowdHost struct {
	addr     string
	resource *http.Client
	alive    bool
}

type ShadowdUpstream struct {
	hosts []*ShadowdHost
}

type HashTableNotFoundError struct {
	error
}

type TokensListNotFoundError struct {
	error
}

func NewShadowdHost(addr string, resource *http.Client) (*ShadowdHost, error) {
	if strings.Contains(addr, "://") {
		return nil, fmt.Errorf("shadowd host must be in the format host:port")
	}

	shadowdHost := &ShadowdHost{
		addr:     addr,
		resource: resource,
		alive:    true,
	}

	return shadowdHost, nil
}

func (shadowdHost *ShadowdHost) SetIsAlive(alive bool) {
	shadowdHost.alive = alive
}

func (shadowdHost *ShadowdHost) IsAlive() bool {
	return shadowdHost.alive
}

func (shadowdHost *ShadowdHost) GetAddr() string {
	return shadowdHost.addr
}

func (shadowdHost *ShadowdHost) GetShadow(
	pool string, user string,
) (*Shadow, error) {
	var token string

	if pool != "" {
		token = pool + "/" + user
	} else {
		token = user
	}

	hash, err := shadowdHost.getHash(token)
	if err != nil {
		return nil, err
	}

	proofHash, err := shadowdHost.getHash(token)
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

	return shadow, nil
}

func (shadowdHost *ShadowdHost) getHash(token string) (string, error) {
	response, err := shadowdHost.resource.Get(
		"https://" + shadowdHost.addr + "/t/" + token,
	)
	if err != nil {
		return "", err
	}

	if response.StatusCode != 200 {
		if response.StatusCode == 404 {
			return "", HashTableNotFoundError{
				fmt.Errorf(
					"hash table for token '%s' not found", token,
				),
			}
		}

		return "", fmt.Errorf("error HTTP status: %s", response.Status)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	response.Body.Close()

	return strings.TrimRight(string(body), "\n"), nil
}

func (shadowdHost *ShadowdHost) GetTokens(base string) ([]string, error) {
	response, err := shadowdHost.resource.Get(
		"https://" + shadowdHost.addr + "/t/" +
			strings.TrimSuffix(base, "/") + "/",
	)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != 200 {
		if response.StatusCode == 404 {
			return nil, TokensListNotFoundError{
				fmt.Errorf(
					"no tokens from '%s' is found", base,
				),
			}
		}

		return nil, fmt.Errorf("error HTTP status: %s", response.Status)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	return strings.Split(string(body), "\n"), nil
}

func NewShadowdUpstream(
	addrs []string, certificateFilepath string,
) (*ShadowdUpstream, error) {
	pemData, err := ioutil.ReadFile(certificateFilepath)
	if err != nil {
		return nil, err
	}

	pemBlock, _ := pem.Decode(pemData)
	if pemBlock == nil {
		return nil, fmt.Errorf(
			"%s is not valid certificate file because PEM data is not found",
			certificateFilepath,
		)
	}

	certificate, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return nil, err
	}

	certsPool := x509.NewCertPool()
	certsPool.AddCert(certificate)

	resource := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certsPool,
			},
		},
	}

	upstream := ShadowdUpstream{}
	for _, addr := range addrs {
		shadowdHost, err := NewShadowdHost(addr, resource)
		if err != nil {
			return nil, err
		}

		upstream.hosts = append(upstream.hosts, shadowdHost)
	}

	return &upstream, nil
}

func (upstream *ShadowdUpstream) GetAliveShadowdHosts() (
	[]*ShadowdHost, error,
) {
	hosts := []*ShadowdHost{}
	for _, host := range upstream.hosts {
		if host.IsAlive() {
			hosts = append(hosts, host)
		}
	}

	if len(hosts) < 0 {
		return nil, fmt.Errorf("no living shadowd hosts")
	}

	return hosts, nil
}
