// Copyright 2020 Northern.tech AS
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

package https

import (
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	pkcs11URIPrefix = "pkcs11:"
)

// Client configuration

// Config holds the configuration for HTTP clients
type Config struct {
	IsHTTPS    bool
	ServerCert string
	*Client
	NoVerify bool
}

// Client holds the configuration for the client side mTLS configuration
// NOTE: Careful when changing this, the struct is exposed directly in the
// 'mender-connect.conf' file.
type Client struct {
	Certificate string
	Key         string
	SSLEngine   string
}

// MenderServer is a placeholder for a full server definition used when
// multiple servers are given. The fields corresponds to the definitions
// given in MenderShellConfig.
type MenderServer struct {
	ServerURL string
}

// Validate validates the Client's configuration
func (h *Client) Validate() {
	if h == nil {
		return
	}
	if h.Certificate != "" || h.Key != "" {
		if h.Certificate == "" {
			log.Error("The 'Key' field is set in the mTLS configuration, but no 'Certificate' is given. Both need to be present in order for mTLS to function")
		}
		if h.Key == "" {
			log.Error("The 'Certificate' field is set in the mTLS configuration, but no 'Key' is given. Both need to be present in order for mTLS to function")
		} else if strings.HasPrefix(h.Key, pkcs11URIPrefix) && len(h.SSLEngine) == 0 {
			log.Errorf("The 'Key' field is set to be loaded from %s, but no 'SSLEngine' is given. Both need to be present in order for loading of the key to function",
				pkcs11URIPrefix)
		}
	}
}
