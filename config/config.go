// Copyright 2021 Northern.tech AS
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

package config

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/mendersoftware/mender-connect/client/https"
)

const httpsSchema = "https"

type TerminalConfig struct {
	Width  uint16
	Height uint16
	// Disable remote terminal
	Disable bool
}

type MenderClientConfig struct {
	// Disable mender-client websocket bindings.
	Disable bool
}

type FileTransferConfig struct {
	// Disable file transfer features
	Disable bool
}

type PortForwardConfig struct {
	// Disable port forwarding feature
	Disable bool
}

type SessionsConfig struct {
	// Whether to stop expired sessions
	StopExpired bool
	// Seconds after startup of a sessions that will make it expire
	ExpireAfter uint32
	// Seconds after last activity of a sessions that will make it expire
	ExpireAfterIdle uint32
	// Max sessions per user
	MaxPerUser uint32
}

// MenderShellConfigFromFile holds the configuration settings read from the config file
type MenderShellConfigFromFile struct {
	// ClientProtocol "https"
	ClientProtocol string
	// HTTPS client parameters
	HTTPSClient https.Client `json:"HttpsClient"`
	// Skip CA certificate validation
	SkipVerify bool
	// Path to server SSL certificate
	ServerCertificate string
	// Server URL (For single server conf)
	ServerURL string
	// List of available servers, to which client can fall over
	Servers []https.MenderServer
	// The command to run as shell
	ShellCommand string
	// Name of the user who owns the shell process
	User string
	// Terminal settings
	Terminal TerminalConfig `json:"Terminal"`
	// User sessions settings
	Sessions SessionsConfig `json:"Sessions"`
	// Reconnect interval
	ReconnectIntervalSeconds int
	// FileTransfer config
	FileTransfer FileTransferConfig
	// PortForward config
	PortForward PortForwardConfig
	// MenderClient config
	MenderClient MenderClientConfig
}

// MenderShellConfig holds the configuration settings for the Mender shell client
type MenderShellConfig struct {
	MenderShellConfigFromFile
	Debug bool
	Trace bool
}

// NewMenderShellConfig initializes a new MenderShellConfig struct
func NewMenderShellConfig() *MenderShellConfig {
	return &MenderShellConfig{
		MenderShellConfigFromFile: MenderShellConfigFromFile{},
	}
}

// LoadConfig parses the mender configuration json-files
// (/etc/mender/mender-connect.conf and /var/lib/mender/mender-connect.conf)
// and loads the values into the MenderShellConfig structure defining high level
// client configurations.
func LoadConfig(mainConfigFile string, fallbackConfigFile string) (*MenderShellConfig, error) {
	// Load fallback configuration first, then main configuration.
	// It is OK if either file does not exist, so long as the other one does exist.
	// It is also OK if both files exist.
	// Because the main configuration is loaded last, its option values
	// override those from the fallback file, for options present in both files.
	var filesLoadedCount int
	config := NewMenderShellConfig()

	if loadErr := loadConfigFile(fallbackConfigFile, config, &filesLoadedCount); loadErr != nil {
		return nil, loadErr
	}

	if loadErr := loadConfigFile(mainConfigFile, config, &filesLoadedCount); loadErr != nil {
		return nil, loadErr
	}

	log.Debugf("Loaded %d configuration file(s)", filesLoadedCount)
	if filesLoadedCount == 0 {
		log.Info("No configuration files present. Using defaults")
		return config, nil
	}

	log.Debugf("Loaded configuration = %#v", config)
	return config, nil
}

func isExecutable(path string) bool {
	info, _ := os.Stat(path)
	if info == nil {
		return false
	}
	mode := info.Mode()
	return (mode & 0111) != 0
}

func isInShells(path string) bool {
	file, err := os.Open("/etc/shells")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	found := false
	for scanner.Scan() {
		if scanner.Text() == path {
			found = true
			break
		}
	}
	return found
}

func validateUser(c *MenderShellConfig) (err error) {
	if c.User == "" {
		return errors.New("please provide a user to run the shell as")
	}
	u, err := user.Lookup(c.User)
	if err == nil && u == nil {
		return errors.New("unknown error while getting a user id")
	}
	if err != nil {
		return err
	}
	return nil
}

// Validate verifies the Servers fields in the configuration
func (c *MenderShellConfig) Validate() (err error) {
	if c.Servers == nil {
		if c.ServerURL == "" {
			log.Warn("No server URL(s) specified in mender configuration.")
		}
		c.Servers = make([]https.MenderServer, 1)
		c.Servers[0].ServerURL = c.ServerURL
	} else if c.ServerURL != "" {
		log.Error("In mender-connect.conf: don't specify both Servers field " +
			"AND the corresponding fields in base structure (i.e. " +
			"ServerURL). The first server on the list overwrites" +
			"these fields.")
		return errors.New("Both Servers AND ServerURL given in " +
			"mender-connect.conf")
	}
	for i, u := range c.Servers {
		_, err := url.Parse(u.ServerURL)
		if err != nil {
			log.Errorf("'%s' at Servers[%d].ServerURL is not a valid URL", u.ServerURL, i)
			return err
		}
	}
	for i := 0; i < len(c.Servers); i++ {
		// trim possible '/' suffix, which is added back in URL path
		if strings.HasSuffix(c.Servers[i].ServerURL, "/") {
			c.Servers[i].ServerURL =
				strings.TrimSuffix(
					c.Servers[i].ServerURL, "/")
		}
		if c.Servers[i].ServerURL == "" {
			log.Warnf("Server entry %d has no associated server URL.", i+1)
		}
	}

	//check if shell is given, if not, defaulting to /bin/sh
	if c.ShellCommand == "" {
		log.Warnf("ShellCommand is empty, defaulting to %s", DefaultShellCommand)
		c.ShellCommand = DefaultShellCommand
	}

	if !filepath.IsAbs(c.ShellCommand) {
		return errors.New("given shell (" + c.ShellCommand + ") is not an absolute path")
	}

	if !isExecutable(c.ShellCommand) {
		return errors.New("given shell (" + c.ShellCommand + ") is not executable")
	}

	err = validateUser(c)
	if err != nil {
		return err
	}

	if !isInShells(c.ShellCommand) {
		log.Errorf("ShellCommand %s is not present in /etc/shells", c.ShellCommand)
		return errors.New("ShellCommand " + c.ShellCommand + " is not present in /etc/shells")
	}

	if c.Terminal.Width == 0 {
		c.Terminal.Width = DefaultTerminalWidth
	}

	if c.Terminal.Height == 0 {
		c.Terminal.Height = DefaultTerminalHeight
	}

	if !c.Sessions.StopExpired {
		c.Sessions.ExpireAfter = 0
		c.Sessions.ExpireAfterIdle = 0
	} else {
		if c.Sessions.ExpireAfter > 0 && c.Sessions.ExpireAfterIdle > 0 {
			log.Warnf("both ExpireAfter and ExpireAfterIdle specified.")
		}
	}

	if c.ReconnectIntervalSeconds == 0 {
		c.ReconnectIntervalSeconds = DefaultReconnectIntervalsSeconds
	}

	c.HTTPSClient.Validate()
	log.Debugf("Verified configuration = %#v", c)

	return nil
}

func loadConfigFile(configFile string, config *MenderShellConfig, filesLoadedCount *int) error {
	// Do not treat a single config file not existing as an error here.
	// It is up to the caller to fail when both config files don't exist.
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		log.Debug("Configuration file does not exist: ", configFile)
		return nil
	}

	if err := readConfigFile(&config.MenderShellConfigFromFile, configFile); err != nil {
		log.Errorf("Error loading configuration from file: %s (%s)", configFile, err.Error())
		return err
	}

	(*filesLoadedCount)++
	log.Info("Loaded configuration file: ", configFile)
	return nil
}

func readConfigFile(config interface{}, fileName string) error {
	// Reads mender configuration (JSON) file.
	log.Debug("Reading Mender configuration from file " + fileName)
	conf, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(conf, &config); err != nil {
		switch err.(type) {
		case *json.SyntaxError:
			return errors.New("Error parsing mender configuration file: " + err.Error())
		}
		return errors.New("Error parsing config file: " + err.Error())
	}

	return nil
}

// maybeHTTPSClient returns the HTTPSClient config only when both
// certificate and key are provided
func maybeHTTPSClient(c *MenderShellConfig) *https.Client {
	c.HTTPSClient.Validate()
	if c.HTTPSClient.Certificate != "" && c.HTTPSClient.Key != "" {
		return &c.HTTPSClient
	}
	return nil
}

// GetHTTPConfig returns the configuration for the HTTP client
func (c *MenderShellConfig) GetHTTPConfig() https.Config {
	return https.Config{
		ServerCert: c.ServerCertificate,
		IsHTTPS:    c.ClientProtocol == httpsSchema,
		Client:     maybeHTTPSClient(c),
		NoVerify:   c.SkipVerify,
	}
}
