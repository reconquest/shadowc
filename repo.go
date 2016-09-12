package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/reconquest/hierr-go"
)

type ShadowdHost struct {
	address  string
	resource *http.Client
	alive    bool
}

type ShadowdUpstream struct {
	hosts []*ShadowdHost
}

type NotFoundError struct {
	error
}

func NewShadowdHost(
	address string, resource *http.Client,
) (*ShadowdHost, error) {
	if strings.Contains(address, "://") {
		return nil, errors.New(
			"shadowd server address must be in host:port format",
		)
	}

	shadowdHost := &ShadowdHost{
		address:  address,
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
	return shadowdHost.address
}

func (shadowdHost *ShadowdHost) GetShadow(
	pool string, username string,
) (*Shadow, error) {
	var token string

	if pool != "" {
		token = pool + "/" + username
	} else {
		token = username
	}

	hash, err := shadowdHost.getHash(token)
	if err != nil {
		if _, ok := err.(NotFoundError); ok {
			return nil, err
		}

		return nil, hierr.Errorf(
			err,
			"can't retrieve shadow entry for %s",
			user{username, pool},
		)
	}

	proofHash, err := shadowdHost.getHash(token)
	if err != nil {
		if _, ok := err.(NotFoundError); ok {
			return nil, err
		}

		return nil, hierr.Errorf(
			err,
			"can't retrieve proofing shadow entry for %s",
			user{username, pool},
		)
	}

	if hash == proofHash {
		warningf(
			"[!] hash for %s was recently requested; "+
				"possible break-in attempt.",
			user{username, pool},
		)
	}

	shadow := &Shadow{
		Username: username,
		Hash:     hash,
	}

	return shadow, nil
}

func (shadowdHost *ShadowdHost) GetSSHKeys(
	pool string, username string,
) (SSHKeys, error) {
	var token string

	if pool != "" {
		token = pool + "/" + username
	} else {
		token = username
	}

	body, err := request(
		shadowdHost.resource,
		"GET", "https://"+shadowdHost.address+"/ssh/"+token,
	)
	if err != nil {
		return nil, err
	}

	sshKeys := SSHKeys{}

	rawKeys := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for keyIndex, rawKey := range rawKeys {
		key, err := ReadSSHKey(rawKey)
		if err != nil {
			return nil, hierr.Errorf(
				err, "error while parsing #%d key", keyIndex+1,
			)
		}

		sshKeys = append(sshKeys, key)
	}

	return sshKeys, nil
}

func (shadowdHost *ShadowdHost) getHash(token string) (string, error) {
	body, err := request(
		shadowdHost.resource,
		"GET", "https://"+shadowdHost.address+"/t/"+token,
	)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(body, "\n"), nil
}

func (shadowdHost *ShadowdHost) GetTokens(base string) ([]string, error) {
	body, err := request(
		shadowdHost.resource,
		"GET",
		"https://"+shadowdHost.address+"/t/"+strings.TrimSuffix(base, "/")+"/",
	)
	if err != nil {
		return nil, err
	}

	return strings.Split(body, "\n"), nil
}

func (shadowdHost *ShadowdHost) GetPasswordChangeSalts(
	pool, username string,
) ([]string, error) {
	var token string

	if pool != "" {
		token = pool + "/" + username
	} else {
		token = username
	}

	body, err := request(
		shadowdHost.resource,
		"PUT",
		"https://"+shadowdHost.address+"/t/"+strings.TrimSuffix(token, "/"),
	)
	if err != nil {
		return nil, err
	}

	return strings.Split(strings.TrimSuffix(body, "\n"), "\n"), nil
}

func (shadowdHost *ShadowdHost) ChangePassword(
	pool, username string, shadows []string, password string,
) error {
	var token string

	if pool != "" {
		token = pool + "/" + username
	} else {
		token = username
	}

	_, err := request(
		shadowdHost.resource,
		"PUT",
		"https://"+shadowdHost.address+"/t/"+strings.TrimSuffix(token, "/"),
		url.Values{
			"shadow[]": shadows,
			"password": []string{password},
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func NewShadowdUpstream(
	addresss []string, certificateFilepath string,
) (*ShadowdUpstream, error) {
	pemData, err := ioutil.ReadFile(certificateFilepath)
	if err != nil {
		return nil, hierr.Errorf(
			err, "can't read certificate file %s", certificateFilepath,
		)
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
		return nil, hierr.Errorf(
			err, "can't parse certificate PEM block",
		)
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
	for _, address := range addresss {
		shadowdHost, err := NewShadowdHost(address, resource)
		if err != nil {
			return nil, hierr.Errorf(
				err, "can't initialize shadowd client for %s", address,
			)
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
		return nil, errors.New("all shadowd servers has gone away")
	}

	return hosts, nil
}

func readHTTPResponse(response *http.Response) (string, error) {
	debugf("%s", response.Status)

	if response.StatusCode != 200 {
		if response.StatusCode == 404 {
			return "", NotFoundError{
				errors.New("404 Not Found"),
			}
		}
		if response.StatusCode == 204 {
			return "", NotFoundError{
				errors.New("204 No Content"),
			}
		}

		return "", fmt.Errorf("unexpected status %s", response.Status)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", hierr.Errorf(
			err, "can't read response body",
		)
	}

	tracef("response: '%s'", string(body))

	return string(body), nil
}

func request(
	client *http.Client,
	method string,
	url string,
	body ...url.Values,
) (string, error) {
	var payload string
	if len(body) > 0 {
		payload = body[0].Encode()
	}

	debugf("%s %s %s", method, url, payload)

	request, err := http.NewRequest(
		method,
		url,
		bytes.NewBufferString(payload),
	)
	if err != nil {
		return "", err
	}

	request.Header.Set("User-Agent", "shadowc/"+version)

	response, err := client.Do(request)
	if err != nil {
		return "", err
	}

	return readHTTPResponse(response)
}
