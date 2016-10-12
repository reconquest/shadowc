package main

import (
	"bufio"
	"io"
	"os"

	"github.com/reconquest/hierr-go"

	"golang.org/x/crypto/ssh"
)

type SSHKey struct {
	Comment string
	Raw     string
}

type SSHKeys []*SSHKey
type AuthorizedKeys map[string]SSHKeys

type AuthorizedKeysFile struct {
	path string
	keys SSHKeys
}

func ReadSSHKey(key string) (*SSHKey, error) {
	_, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
	if err != nil {
		return nil, hierr.Errorf(
			err, "can't parse authorized key",
		)
	}

	return &SSHKey{
		Comment: comment,
		Raw:     key,
	}, nil
}

func (key *SSHKey) GetComment() string {
	return key.Comment
}

func NewAuthorizedKeysFile(path string) *AuthorizedKeysFile {
	return &AuthorizedKeysFile{
		path: path,
	}
}

func ReadAuthorizedKeysFile(path string) (*AuthorizedKeysFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, hierr.Errorf(
			err, "can't open authorized keys file",
		)
	}

	sshKeys := SSHKeys{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, err := ReadSSHKey(scanner.Text())
		if err != nil {
			return nil, err
		}

		sshKeys = append(sshKeys, key)
	}

	return &AuthorizedKeysFile{
		path: path,
		keys: sshKeys,
	}, nil
}

func (file *AuthorizedKeysFile) AddSSHKey(key *SSHKey) bool {
	for _, existKey := range file.keys {
		if existKey.Raw == key.Raw {
			return false
		}
	}

	file.keys = append(file.keys, key)

	return true
}

func (file *AuthorizedKeysFile) Write(writer io.Writer) (int, error) {
	totalWritten := 0

	for _, key := range file.keys {
		written, err := io.WriteString(writer, string(key.Raw)+"\n")
		if err != nil {
			return totalWritten, err
		}

		totalWritten += written
	}

	return totalWritten, nil
}

func (file *AuthorizedKeysFile) GetPath() string {
	return file.path
}
