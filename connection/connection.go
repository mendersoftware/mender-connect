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

package connection

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack"

	"github.com/mendersoftware/go-lib-micro/ws"
)

const (
	errMissingServerCertF = "IGNORING ERROR: The client server-certificate can not be " +
		"loaded: (%s). The client will continue running, but may not be able to " +
		"communicate with the server. If this is not your intention please add a valid " +
		"server certificate"
	errMissingCerts = "No trusted certificates. The client will continue running, but will " +
		"not be able to communicate with the server. Either specify ServerCertificate in " +
		"mender.conf, or make sure that CA certificates are installed on the system"
)

type Connection struct {
	writeMutex sync.Mutex
	// the connection handler
	connection *websocket.Conn
	// Time allowed to write a message to the peer.
	writeWait time.Duration
	// Maximum message size allowed from peer.
	maxMessageSize int64
	// Time allowed to read the next pong message from the peer.
	defaultPingWait time.Duration
}

func loadServerTrust(serverCertFilePath string) *x509.CertPool {
	systemPool, err := x509.SystemCertPool()
	if err != nil {
		log.Warnf("Error when loading system certificates: %s", err.Error())
	}

	if systemPool == nil {
		log.Warn("No system certificates found.")
		systemPool = x509.NewCertPool()
	}

	if len(serverCertFilePath) < 1 {
		log.Warnf(errMissingServerCertF, "no file provided")
		return systemPool
	}

	log.Infof("loadServerTrust loading certificate from %s", serverCertFilePath)
	// Read certificate file.
	serverCertificate, err := ioutil.ReadFile(serverCertFilePath)
	if err != nil {
		// Ignore server certificate error  (See: MEN-2378)
		log.Warnf(errMissingServerCertF, err.Error())
	}

	if len(serverCertificate) > 0 {
		block, _ := pem.Decode(serverCertificate)
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				log.Infof("API Gateway certificate (in PEM format): \n%s", string(serverCertificate))
				log.Infof("Issuer: %s, Valid from: %s, Valid to: %s",
					cert.Issuer.Organization, cert.NotBefore, cert.NotAfter)
			} else {
				log.Warnf("Unparseable certificate '%s': %s", serverCertFilePath, err.Error())
			}
		}

		systemPool.AppendCertsFromPEM(serverCertificate)
	}

	if len(systemPool.Subjects()) == 0 {
		log.Error(errMissingCerts)
	}
	return systemPool
}

//Websocket connection routine. setup the ping-pong and connection settings
func NewConnection(u url.URL,
	token string,
	writeWait time.Duration,
	maxMessageSize int64,
	defaultPingWait time.Duration,
	skipVerify bool,
	serverCertFilePath string) (*Connection, error) {
	// skip verification of HTTPS certificate if skipVerify is set in the config file

	websocket.DefaultDialer.TLSClientConfig = &tls.Config{
		RootCAs:            loadServerTrust(serverCertFilePath),
		InsecureSkipVerify: skipVerify,
	}

	var ws *websocket.Conn
	dialer := *websocket.DefaultDialer

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	ws, _, err := dialer.Dial(u.String(), headers)
	if err != nil {
		return nil, err
	}

	c := &Connection{
		connection:      ws,
		writeWait:       writeWait,
		maxMessageSize:  maxMessageSize,
		defaultPingWait: defaultPingWait,
	}
	// ping-pong
	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(defaultPingWait))
	ws.SetPingHandler(func(message string) error {
		pongWait, _ := strconv.Atoi(message)
		ws.SetReadDeadline(time.Now().Add(time.Duration(pongWait) * time.Second))
		c.writeMutex.Lock()
		defer c.writeMutex.Unlock()
		return ws.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(writeWait))
	})
	return c, nil
}

func (c *Connection) GetWriteTimeout() time.Duration {
	return c.writeWait
}

func (c *Connection) WriteMessage(m *ws.ProtoMsg) (err error) {
	data, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()
	c.connection.SetWriteDeadline(time.Now().Add(c.writeWait))
	return c.connection.WriteMessage(websocket.BinaryMessage, data)
}

// keeping those for debugging and internal use
func (c *Connection) writeMessageRaw(data []byte) (err error) {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()
	c.connection.SetWriteDeadline(time.Now().Add(c.writeWait))
	return c.connection.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Connection) ReadMessage() (*ws.ProtoMsg, error) {
	_, data, err := c.connection.ReadMessage()
	if err != nil {
		return nil, err
	}

	m := &ws.ProtoMsg{}
	err = msgpack.Unmarshal(data, m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// keeping those for debugging and internal use
func (c *Connection) readMessageRaw() ([]byte, error) {
	_, data, err := c.connection.ReadMessage()
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (c *Connection) Close() error {
	return c.connection.Close()
}
