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
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
)

const testConfig = `{
  "User":"root",
  "Terminal": {
    "Height": 80,
    "Width": 24
  },
  "Sessions": {
    "StopExpired": true,
    "ExpireAfter": 16,
    "ExpireAfterIdle": 8,
    "MaxPerUser": 4
  }
}`

const testBrokenConfig = `{
  "User":"root"
  "Terminal": {
    "Height": 80,
    "Width": 24
  },
}`

const testShellIsNotAbsolutePathConfig = `{
		"ShellCommand": "bash",
        "User": "root"
}`

const testShellIsNotExecutablePathConfig = `{
		"ShellCommand": "/etc/profile",
        "User": "root"
}`

const testShellIsNotPresentConfig = `{
		"ShellCommand": "/most/not/here/now",
        "User": "root"
}`

const testUserNotFoundConfig = `{
		"ShellCommand": "/bin/bash",
        "User": "thisoneisnotknown"
}`

const testShellNotInShellsConfig = `{
		"ShellCommand": "/bin/ls",
        "User": "root"
}`

const testParsingErrorConfig = `{
		"ShellCommand": "/bin/ls"
        "User": "root"
}`

const testOtherErrorConfig = `{
		"ShellCommand": 0.0e14,
        "User": "root"
}`

func Test_readConfigFile_noFile_returnsError(t *testing.T) {
	err := readConfigFile(nil, "non-existing-file")
	assert.Error(t, err)
}

func Test_readConfigFile_brokenContent_returnsError(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)

	configFile.WriteString(testBrokenConfig)

	// fail on first call to readConfigFile (invalid JSON)
	confFromFile, err := LoadConfig(configPath, "does-not-exist.config")
	assert.Error(t, err)
	assert.Nil(t, confFromFile)

	// fail on second call to readConfigFile (invalid JSON)
	confFromFile, err = LoadConfig("does-not-exist.config", configPath)
	assert.Error(t, err)
	assert.Nil(t, confFromFile)
}

func validateConfiguration(t *testing.T, actual *MenderShellConfig) {
	expectedConfig := NewMenderShellConfig()
	expectedConfig.MenderShellConfigFromFile = MenderShellConfigFromFile{
		User:           "root",
		ShellCommand:   DefaultShellCommand,
		ShellArguments: DefaultShellArguments,
		Terminal: TerminalConfig{
			Width:  24,
			Height: 80,
		},
		Sessions: SessionsConfig{
			StopExpired:     true,
			ExpireAfter:     16,
			ExpireAfterIdle: 8,
			MaxPerUser:      4,
		},
		ReconnectIntervalSeconds: DefaultReconnectIntervalsSeconds,
		Limits: Limits{
			Enabled: false,
			FileTransfer: FileTransferLimits{
				Chroot:         "",
				FollowSymLinks: false,
				AllowOverwrite: false,
				OwnerPut:       "",
				GroupPut:       "",
				Umask:          "",
				MaxFileSize:    0,
				Counters: RateLimits{
					MaxBytesTxPerMinute: 0,
					MaxBytesRxPerMinute: 0,
				},
				AllowSuid:        false,
				RegularFilesOnly: false,
				PreserveMode:     true,
				PreserveOwner:    true,
			},
		},
	}
	if !assert.True(t, reflect.DeepEqual(actual, expectedConfig)) {
		t.Logf("got:      %+v", actual)
		t.Logf("expected: %+v", expectedConfig)
	}
}

func Test_LoadConfig_correctConfFile_returnsConfiguration(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testConfig)

	// fallback configuration file does not exist
	config, err := LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.NoError(t, err)
	validateConfiguration(t, config)
	assert.Equal(t, "root", config.User)

	// main configuration file does not exist
	config2, err2 := LoadConfig("does-not-exist.config", configPath)
	assert.NoError(t, err2)
	assert.NotNil(t, config2)
	err = config2.Validate()
	assert.NoError(t, err)
	validateConfiguration(t, config2)
	assert.Equal(t, "root", config.User)
}

func TestConfigurationMergeSettings(t *testing.T) {
	var mainConfigJSON = `{
		"ShellCommand": "/bin/bash",
		"ReconnectIntervalSeconds": 123
	}`

	var fallbackConfigJSON = `{
		"ShellCommand": "/bin/sh",
		"User": "mender"
	}`

	mainConfigFile, err := os.Create("main.config")
	assert.NoError(t, err)
	defer os.Remove("main.config")
	mainConfigFile.WriteString(mainConfigJSON)

	fallbackConfigFile, err := os.Create("fallback.config")
	assert.NoError(t, err)
	defer os.Remove("fallback.config")
	fallbackConfigFile.WriteString(fallbackConfigJSON)

	config, err := LoadConfig("main.config", "fallback.config")
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// when a setting appears in neither file, it is left with its default value.
	assert.Equal(t, uint16(0), config.Terminal.Width)
	assert.Equal(t, uint16(0), config.Terminal.Height)

	// when a setting appears in both files, the main file takes precedence.
	assert.Equal(t, "/bin/bash", config.ShellCommand)

	// when a setting appears in only one file, its value is used.
	assert.Equal(t, "mender", config.User)
	assert.Equal(t, 123, config.ReconnectIntervalSeconds)
}

func Test_LoadConfig_various_errors(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	//one of the serverURLs is not valid
	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)

	//shell is not an absolute path
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testShellIsNotAbsolutePathConfig)

	config, err := LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//shell is not executable
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testShellIsNotExecutablePathConfig)

	config, err = LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//shell is not present
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testShellIsNotPresentConfig)

	config, err = LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//user not found
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testUserNotFoundConfig)

	config, err = LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//shell is not found in /etc/shells
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testShellNotInShellsConfig)

	config, err = LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//parsing error
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testParsingErrorConfig)

	_, err = LoadConfig(configPath, "")
	assert.Error(t, err)

	//other loading error
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testOtherErrorConfig)

	_, err = LoadConfig(configPath, "")
	assert.Error(t, err)
}

func TestConfigurationNeitherFileExistsIsNotError(t *testing.T) {
	config, err := LoadConfig("does-not-exist", "also-does-not-exist")
	assert.NoError(t, err)
	assert.IsType(t, &MenderShellConfig{}, config)
}

func TestShellArgumentsEmptyDefaults(t *testing.T) {
	// Test default shell arguments
	var mainConfigJSON = `{
		"ShellCommand": "/bin/bash"
	}`

	mainConfigFile, err := os.Create("main.config")
	assert.NoError(t, err)
	defer os.Remove("main.config")
	mainConfigFile.WriteString(mainConfigJSON)

	config, err := LoadConfig("main.config", "")
	assert.NoError(t, err)
	assert.NotNil(t, config)

	config.Validate()

	assert.Equal(t, []string{"--login"}, config.ShellArguments)

	// Test empty shell arguments
	mainConfigJSON = `{
		"ShellCommand": "/bin/bash",
	        "ShellArguments": [""]
	}`

	mainConfigFile, err = os.Create("main.config")
	assert.NoError(t, err)
	defer os.Remove("main.config")
	mainConfigFile.WriteString(mainConfigJSON)

	config, err = LoadConfig("main.config", "")
	assert.NoError(t, err)

	config.Validate()

	assert.Equal(t, []string{""}, config.ShellArguments)

	// Test setting custom shell arguments
	mainConfigJSON = `{
		"ShellCommand": "/bin/bash",
	        "ShellArguments": ["--no-profile", "--norc", "--restricted"]
	}`

	mainConfigFile, err = os.Create("main.config")
	assert.NoError(t, err)
	defer os.Remove("main.config")
	mainConfigFile.WriteString(mainConfigJSON)

	config, err = LoadConfig("main.config", "")
	assert.NoError(t, err)

	config.Validate()

	assert.Equal(t, []string{"--no-profile", "--norc", "--restricted"}, config.ShellArguments)

}

func TestServerArgumentsDeprecated(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	configFile.WriteString(`{"ServerURL": "https://mender.io/"}`)
	_, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "ServerURL field is deprecated")

	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(`{"Servers": [{"ServerURL": "https://hosted.mender.io"}]}`)
	buf.Reset()
	_, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Servers field is deprecated")

	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(`{
		"ServerURL": "https://hosted.mender.io",
		"User":"root",
		"ShellCommand": "/bin/bash",
		"Servers": [{"ServerURL": "https://hosted.mender.io"}]
	  }`)
	buf.Reset()
	_, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "ServerURL field is deprecated")
	assert.Contains(t, buf.String(), "Servers field is deprecated")

	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(`{"ClientProtocol": "something"}`)
	buf.Reset()
	_, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "ClientProtocol field is deprecated")

	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(`{"HTTPSClient": {"Certificate": "client.crt"}}`)
	buf.Reset()
	_, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "HTTPSClient field is deprecated")

	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(`{"HTTPSClient": {"Key": "key.secret"}}`)
	buf.Reset()
	_, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "HTTPSClient field is deprecated")

	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(`{"HTTPSClient": {"SSLEngine": "engine.power"}}`)
	buf.Reset()
	_, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "HTTPSClient field is deprecated")

	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(`{"SkipVerify": true}`)
	buf.Reset()
	_, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "SkipVerify field is deprecated")

	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(`{"ServerCertificate": "certificate.crt"}`)
	buf.Reset()
	_, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "ServerCertificate field is deprecated")
}
