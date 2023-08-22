// Copyright 2021 Northern.tech AS
//
//	Licensed under the Apache License, Version 2.0 (the "License");
//	you may not use this file except in compliance with the License.
//	You may obtain a copy of the License at
//
//	    http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS,
//	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	See the License for the specific language governing permissions and
//	limitations under the License.

package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/mendersoftware/mender-connect/config"
	cryptoutil "github.com/mendersoftware/mender-connect/utils/crypto"
	log "github.com/sirupsen/logrus"
)

func bootstrap(c *cli.Context, cfg *config.MenderShellConfig) error {
	var err error
	switch cfg.APIConfig.APIType {
	case config.APITypeHTTP:
		if _, err = os.Stat(cfg.APIConfig.PrivateKey); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("unexpected error checking file existance: %w", err)
		}
		if os.IsNotExist(err) || c.Bool("force") {
			kt, err := cryptoutil.ParseKeyType(c.String("key-type"))
			if err != nil {
				return err
			}
			err = cryptoutil.GeneratePrivateKeyFile(kt, cfg.APIConfig.PrivateKey)
			if err != nil {
				return fmt.Errorf("failed to generate private key: %w", err)
			}
		}
		if _, err = os.Stat(cfg.APIConfig.IdentityData); err != nil &&
			!os.IsNotExist(err) {
			return fmt.Errorf("unexpected error checking file existance: %w", err)
		}
		if os.IsNotExist(err) || c.Bool("force") {
			err = generateIdentityData(
				cfg.APIConfig.IdentityData,
				c.StringSlice("extra-identity"),
			)
			if err != nil {
				return fmt.Errorf("failed to generate private key: %w", err)
			}
		}
	case config.APITypeDBus:
		log.Info("Authentication configured for DBus: skipping bootstrap")

	default:
		err = fmt.Errorf(
			"unknown auth type %q: skipping bootstrap",
			cfg.APIConfig.APIType,
		)
	}
	return err
}

func generateIdentityData(path string, extraValues []string) error {
	var (
		err          error
		iface        net.Interface
		identityData = make(map[string]string, len(extraValues)+1)
	)
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("failed to get interfaces: %w", err)
	}
	for _, iface = range interfaces {
		if iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) > 0 {
			continue
		}
		identityData["mac"] = iface.HardwareAddr.String()
		break
	}

	for _, val := range extraValues {
		idx := strings.IndexByte(val, '=')
		if idx < 0 {
			return fmt.Errorf(
				"malformed identity key/value pair: expected format: `key=value`",
			)
		}
		identityData[val[:idx]] = val[idx+1:]
	}

	const (
		edgeEnvHostName = "IOTEDGE_IOTHUBHOSTNAME"
		edgeEnvDeviceID = "IOTEDGE_DEVICEID"
		edgeEnvModuleID = "IOTEDGE_MODULEID"
	)
	if edgeHost, ok := os.LookupEnv(edgeEnvHostName); ok {
		identityData["iothub:hostname"] = edgeHost
		if deviceID, ok := os.LookupEnv(edgeEnvDeviceID); ok {
			identityData["iothub:device_id"] = deviceID
		}
		if moduleID, ok := os.LookupEnv(edgeEnvModuleID); ok {
			identityData["iothub:module_id"] = moduleID
		}
	}

	fd, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to create identity file: %w", err)
	}
	defer fd.Close()

	enc := json.NewEncoder(fd)
	enc.SetIndent("", "  ")
	err = enc.Encode(identityData)
	if err != nil {
		return fmt.Errorf("error serializing identity data: %w", err)
	}
	return nil
}
