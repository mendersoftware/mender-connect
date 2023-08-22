// Copyright 2023 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package mender

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mendersoftware/mender-connect/client/dbus"
	"github.com/mendersoftware/mender-connect/config"
)

const timeout = 10 * time.Second

// AuthClient is the interface for the Mender Authentication Manager clilents
type AuthClient interface {
	// Connect to the Mender client interface
	Connect(objectName, objectPath, interfaceName string) error
	// GetJWTToken returns a device JWT token
	GetJWTToken() (string, string, error)
	// FetchJWTToken schedules the fetching of a new device JWT token
	FetchJWTToken() (bool, error)
	// WaitForJwtTokenStateChange synchronously waits for the JwtTokenStateChange signal
	WaitForJwtTokenStateChange() ([]dbus.SignalParams, error)
}

// AuthClientDBUS is the implementation of the client for the Mender
// Authentication Manager which communicates using DBUS
type LocalAuthClient struct {
	ServerURL  string        `json:"-"`
	PrivateKey crypto.Signer `json:"-"`

	IDData      string `json:"id_data"`
	TenantToken string `json:"tenant_token,omitempty"`
	ExternalID  string `json:"external_id"`
	PublicKey   string `json:"pubkey"`

	jwt         string
	client      *http.Client
	tokenChange chan struct{}
}

// NewAuthClient returns a new AuthClient
func NewAuthClient(
	cfg config.APIConfig,
) (*LocalAuthClient, error) {
	if cfg.GetPrivateKey() == nil {
		return nil, fmt.Errorf("invalid client config: empty private key")
	} else if len(cfg.GetIdentityData()) == 0 {
		return nil, fmt.Errorf("invalid client config: empty identity data")
	}
	var localAuth = LocalAuthClient{
		ServerURL:   cfg.ServerURL,
		TenantToken: cfg.TenantToken,
		ExternalID:  cfg.ExternalID,
		PrivateKey:  cfg.GetPrivateKey(),
		client:      &http.Client{},
		tokenChange: make(chan struct{}, 1),
	}
	localAuth.tokenChange <- struct{}{}
	identity, err := json.Marshal(cfg.GetIdentityData())
	if err != nil {
		return nil, fmt.Errorf("failed to serialize IDData: %w", err)
	}
	localAuth.IDData = string(identity)

	pub, err := x509.MarshalPKIXPublicKey(localAuth.PrivateKey.Public())
	if err != nil {
		return nil, err
	}
	localAuth.PublicKey = strings.TrimRight(string(pem.EncodeToMemory(
		&pem.Block{Type: "PUBLIC KEY", Bytes: pub},
	)), "\r\n")
	return &localAuth, nil
}

// Connect to the Mender client interface
func (a *LocalAuthClient) Connect(objectName, objectPath, interfaceName string) error {
	_, err := a.FetchJWTToken()
	return err
}

// GetJWTToken returns a device JWT token and server URL
func (a *LocalAuthClient) GetJWTToken() (string, string, error) {
	return a.jwt, a.ServerURL, nil
}

// FetchJWTToken schedules the fetching of a new device JWT token
func (a *LocalAuthClient) FetchJWTToken() (bool, error) {
	const APIURLAuth = "/api/devices/v1/authentication/auth_requests"
	bodyBytes, _ := json.Marshal(a)

	dgst := sha256.Sum256(bodyBytes)
	sig, err := a.PrivateKey.Sign(rand.Reader, dgst[:], crypto.SHA256)
	if err != nil {
		return false, err
	}
	sig64 := base64.StdEncoding.EncodeToString(sig)

	url := strings.TrimRight(a.ServerURL, "/") + APIURLAuth
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Men-Signature", sig64)
	rsp, err := a.client.Do(req)
	if err != nil {
		return false, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode >= 300 {
		return false, fmt.Errorf("unexpected status code: %d", rsp.StatusCode)
	}
	b, err := io.ReadAll(rsp.Body)
	if err != nil {
		return false, err
	}
	a.jwt = string(b)
	return true, nil
}

// WaitForJwtTokenStateChange synchronously waits for the JwtTokenStateChange signal
func (a *LocalAuthClient) WaitForJwtTokenStateChange() ([]dbus.SignalParams, error) {
	<-a.tokenChange
	return nil, nil
}
