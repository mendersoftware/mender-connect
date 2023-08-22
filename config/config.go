// Copyright 2022 Northern.tech AS
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
	"bytes"
	"crypto"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/user"
	"path/filepath"

	cryptoutils "github.com/northerntechhq/nt-connect/utils/crypto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

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

// Counter for the limits  and restrictions for the File Transfer
// on and off the device(MEN-4325)
type RateLimits struct {
	// Maximum bytes count allowed to transfer per minute
	// this is per device global limit, which is consulted
	// every time there is a transfer starting. if above
	// the limit, we answer with error message indicating
	// limit reached.
	MaxBytesTxPerMinute uint64
	MaxBytesRxPerMinute uint64
}

// Limits and restrictions for the File Transfer on and off the device(MEN-4325)
type FileTransferLimits struct {
	// the global parent directory that File Transfer will never escape
	Chroot string
	// No way to escape Chroot, even if this one is set the Chroot setting will
	// be checked for the target of any link and restricted accordingly
	FollowSymLinks bool
	// Allow overwrite files
	AllowOverwrite bool
	// set the owner of new files to OwnerPut
	OwnerPut string
	// set the owner of new files to OwnerPut
	GroupPut string
	// allow to get only files owned by OwnerGet
	OwnerGet []string
	// allow to get only files owned by OwnerGet
	GroupGet []string
	// umask for new files
	Umask string
	// Maximum allowed file size
	MaxFileSize uint64
	// File transfer rate limits
	Counters RateLimits
	// If true it is allowed to upload files with set user id on execute bit set
	AllowSuid bool
	// By default we only allow to send/put regular files
	RegularFilesOnly bool
	// By default we preserve the file modes but set one according to
	//the current umask or configured Umask above
	PreserveMode bool
	// By default we preserve the owner of the file uploaded
	PreserveOwner bool
}

type Limits struct {
	Enabled      bool               `json:"Enabled"`
	FileTransfer FileTransferLimits `json:"FileTransfer"`
}

// MenderShellConfigFromFile holds the configuration settings read from the config file
type MenderShellConfigFromFile struct {
	// The command to run as shell
	ShellCommand string
	// ShellArguments is the arguments the shell is launched with. Defaults
	// to '--login'.
	ShellArguments []string
	// Name of the user who owns the shell process
	User string
	// Terminal settings
	Terminal TerminalConfig `json:"Terminal"`
	// User sessions settings
	Sessions SessionsConfig `json:"Sessions"`
	// Limits and restrictions
	Limits Limits `json:"Limits"`
	// Reconnect interval
	ReconnectIntervalSeconds int
	// FileTransfer config
	FileTransfer FileTransferConfig
	// PortForward config
	PortForward PortForwardConfig
	// MenderClient config
	APIConfig    APIConfig `json:"API"`
	MenderClient MenderClientConfig
	Chroot       string `json:"Chroot"`
}

type APIType string

const (
	APITypeHTTP = "http"
	APITypeDBus = "dbus"
)

func (t APIType) Validate() error {
	switch t {
	case APITypeHTTP, APITypeDBus:
		return nil
	default:
	}
	return fmt.Errorf("invalid auth type %q", t)
}

type APIConfig struct {
	APIType      `json:"Type"`
	ServerURL    string `json:"ServerURL"`
	PrivateKey   string `json:"PrivateKey"`
	IdentityData string `json:"IdentityData"`
	TenantToken  string `json:"TenantToken"`
	ExternalID   string `json:"ExternalID"`

	privateKey   crypto.Signer
	identityData map[string]string
}

func (cfg *APIConfig) load() error {
	const MaxFileSize = 512 * 1024

	if cfg.APIType != APITypeHTTP {
		return nil
	}

	fd, err := os.Open(cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to open private key file: %w", err)
	}
	r := io.LimitReader(fd, MaxFileSize)
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	_ = fd.Close()
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}
	pkey, err := cryptoutils.LoadPrivateKey(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}
	cfg.privateKey = pkey
	buf.Reset()
	fd, err = os.Open(cfg.IdentityData)
	if err != nil {
		return fmt.Errorf("failed to open identity data file: %w", err)
	}
	r = io.LimitReader(fd, MaxFileSize)
	_, err = buf.ReadFrom(r)
	_ = fd.Close()
	if err != nil {
		return fmt.Errorf("failed to read identity data file: %w", err)
	}
	err = json.Unmarshal(buf.Bytes(), &cfg.identityData)
	if err != nil {
		return fmt.Errorf("failed to deserialize identity data: %w", err)
	}
	return nil
}

func (cfg APIConfig) Validate() error {
	err := cfg.load()
	if err != nil {
		return err
	}
	if cfg.APIType == APITypeHTTP {
		if cfg.ServerURL == "" {
			err = fmt.Errorf("empty value")
		} else {
			_, err = url.Parse(cfg.ServerURL)
		}
		if err != nil {
			return fmt.Errorf("invalid ServerURL: %w", err)
		}
		if cfg.TenantToken == "" {
			log.Warn("TenantToken is empty: this device may not show up in your acount")
		}
	}
	return nil
}

func (cfg *APIConfig) GetPrivateKey() crypto.Signer {
	return cfg.privateKey
}

func (cfg *APIConfig) GetIdentityData() map[string]string {
	return cfg.identityData
}

// MenderShellConfigDeprecated holds the deprecated configuration settings
type MenderShellConfigDeprecated struct {
	// Server URL (For single server conf)
	ServerURL string
	// List of available servers, to which client can fall over
	Servers []struct {
		ServerURL string
	}
	// ClientProtocol "https"
	ClientProtocol string
	// HTTPS client parameters
	HTTPSClient struct {
		Certificate string
		Key         string
		SSLEngine   string
	} `json:"HttpsClient"`
	// Skip CA certificate validation
	SkipVerify bool
	// Path to server SSL certificate
	ServerCertificate string
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
		// if no /etc/shell is found, DefaultShellCommand is accepted
		if path == DefaultShellCommand {
			return true
		}
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

func (c *MenderShellConfig) applyDefaults() error {
	//check if shell is given, if not, defaulting to /bin/sh
	if c.ShellCommand == "" {
		log.Warnf("ShellCommand is empty, defaulting to %s", DefaultShellCommand)
		c.ShellCommand = DefaultShellCommand
	}

	if c.ShellArguments == nil {
		log.Warnf("ShellArguments is empty, defaulting to %s", DefaultShellArguments)
		c.ShellArguments = DefaultShellArguments
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

	// permit by default, probably will be changed after integration test is modified
	c.Limits.FileTransfer.PreserveMode = true
	c.Limits.FileTransfer.PreserveOwner = true

	return nil
}

// Validate verifies the Servers fields in the configuration
func (c *MenderShellConfig) Validate() (err error) {
	if err = c.applyDefaults(); err != nil {
		return err
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

	if err := checkforDeprecatedFields(configFile); err != nil {
		log.Errorf("Error loading configuration from file: %s (%s)", configFile, err.Error())
		return err
	}

	*filesLoadedCount++
	log.Info("Loaded configuration file: ", configFile)
	return nil
}

func checkforDeprecatedFields(configFile string) error {
	var deprecatedFields MenderShellConfigDeprecated
	if err := readConfigFile(&deprecatedFields, configFile); err != nil {
		log.Errorf("Error loading configuration from file: %s (%s)", configFile, err.Error())
		return err
	}

	if deprecatedFields.ServerURL != "" {
		log.Warn("ServerURL field is deprecated, ignoring.")
	}
	if len(deprecatedFields.Servers) > 0 {
		log.Warn("Servers field is deprecated, ignoring.")
	}
	if deprecatedFields.ClientProtocol != "" {
		log.Warn("ClientProtocol field is deprecated, ignoring.")
	}
	if deprecatedFields.HTTPSClient.Certificate != "" ||
		deprecatedFields.HTTPSClient.Key != "" ||
		deprecatedFields.HTTPSClient.SSLEngine != "" {
		log.Warn("HTTPSClient field is deprecated, ignoring.")
	}
	if deprecatedFields.SkipVerify {
		log.Warn("SkipVerify field is deprecated, ignoring.")
	}
	if deprecatedFields.ServerCertificate != "" {
		log.Warn("ServerCertificate field is deprecated, ignoring.")
	}

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
