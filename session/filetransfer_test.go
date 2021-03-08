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

package session

import (
	"bytes"
	"github.com/mendersoftware/mender-connect/config"
	"github.com/mendersoftware/mender-connect/limits/filetransfer"
	"github.com/mendersoftware/mender-connect/session/model"
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/mendersoftware/go-lib-micro/ws"
	wsft "github.com/mendersoftware/go-lib-micro/ws/filetransfer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
)

func TestFileTransferUpload(t *testing.T) {
	t.Parallel()
	fileSize := int64(1024)
	testdir, err := ioutil.TempDir("", "filetransfer-testing")
	if err != nil {
		panic(err)
	}
	t.Cleanup(func() { os.RemoveAll(testdir) })
	testCases := []struct {
		Name string

		Params model.FileInfo

		// TransferMessages, if set, are sent before file contents
		TransferMessages []*ws.ProtoMsg
		FileContents     []byte
		ChunkSize        int

		LimitsEnabled bool
		Limits        config.FileTransferLimits

		WriteError error

		Error error
	}{
		{
			Name: "ok",

			Params: model.FileInfo{
				Path: func() *string {
					p := path.Join(testdir, "mkay")
					return &p
				}(),
			},

			FileContents: []byte(
				"this message will be chunked into byte chunks to " +
					"make things super inefficient",
			),
			ChunkSize: 1,
		},
		{
			Name: "error, file too big upload denied",

			Params: model.FileInfo{
				Path: func() *string {
					p := path.Join(testdir, "mkay")
					return &p
				}(),
				Size: &fileSize,
			},

			FileContents: []byte(
				"this message will be chunked into byte chunks to " +
					"make things super inefficient",
			),
			ChunkSize: 1,

			LimitsEnabled: true,
			Limits: config.FileTransferLimits{
				FollowSymLinks: true,
				MaxFileSize:    1,
			},

			Error: errors.New("no file transfer in progress"), //for some reason Upload currently returns this error
		},
		{
			Name: "error, transfer limit reached",

			Params: model.FileInfo{
				Path: func() *string {
					p := path.Join(testdir, "mkay")
					return &p
				}(),
				Size: &fileSize,
			},

			FileContents: []byte(
				"this message will be chunked into byte chunks to " +
					"make things super inefficient",
			),
			ChunkSize: 1,

			LimitsEnabled: true,
			Limits: config.FileTransferLimits{
				FollowSymLinks: true,
				Counters: config.RateLimits{
					MaxBytesTxPerMinute: 1,
					MaxBytesRxPerMinute: 1,
				},
			},

			Error: errors.New("no file transfer in progress"), //for some reason Upload currently returns this error
		},
		{
			Name: "error, fake error from client",

			Params: model.FileInfo{
				Path: func() *string {
					p := path.Join(testdir, "clienterr")
					return &p
				}(),
			},
			TransferMessages: func() []*ws.ProtoMsg {
				errMsg := "something unexpected happened"
				b, _ := msgpack.Marshal(wsft.Error{
					Error: &errMsg,
				})
				return []*ws.ProtoMsg{{
					Header: ws.ProtoHdr{
						Proto:     ws.ProtoTypeFileTransfer,
						MsgType:   wsft.MessageTypeError,
						SessionID: "12344",
					},
					Body: b,
				}}
			}(),
			// We don't expect an error in return here, only a single ack
			// so that it follows the same test path as a successful
			// file transfer.
		}, {
			Name: "error, unexpected ACK message from client",

			Params: model.FileInfo{
				Path: func() *string {
					p := path.Join(testdir, "ackerr")
					return &p
				}(),
			},
			TransferMessages: []*ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeACK,
					SessionID: "12344",
				},
			}},
			Error: errors.New("received unexpected message type 'ack' " +
				"during file upload"),
		}, {
			Name: "error, chunk missing offset",

			Params: model.FileInfo{
				Path: func() *string {
					p := path.Join(testdir, "offseterr")
					return &p
				}(),
			},
			TransferMessages: func() []*ws.ProtoMsg {
				return []*ws.ProtoMsg{{
					Header: ws.ProtoHdr{
						Proto:     ws.ProtoTypeFileTransfer,
						MsgType:   wsft.MessageTypeChunk,
						SessionID: "12344",
					},
					Body: []byte("data"),
				}}
			}(),
			Error: errors.New("invalid file chunk message: missing offset property"),
		}, {
			Name: "error, offset jumps beyond EOF",

			Params: model.FileInfo{
				Path: func() *string {
					p := path.Join(testdir, "badOffset")
					return &p
				}(),
			},
			TransferMessages: func() []*ws.ProtoMsg {
				return []*ws.ProtoMsg{{
					Header: ws.ProtoHdr{
						Proto:     ws.ProtoTypeFileTransfer,
						MsgType:   wsft.MessageTypeChunk,
						SessionID: "12344",
						Properties: map[string]interface{}{
							"offset": int64(1234),
						},
					},
					Body: []byte("data"),
				}}
			}(),
			Error: errors.New("received unexpected chunk offset"),
		}, {
			Name: "error, broken response writer",

			Params: model.FileInfo{
				Path: func() *string {
					p := path.Join(testdir, "errfile")
					return &p
				}(),
			},

			WriteError: io.ErrClosedPipe,
		}, {
			Name: "error, parent directory does not exist",

			Params: model.FileInfo{
				Path: func() *string {
					p := path.Join(
						testdir, "parent", "dir",
						"does", "not", "exist",
						"for", "this", "file",
					)
					return &p
				}(),
			},

			Error: errors.New("failed to create file"),
		}}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			recorder := NewTestWriter(tc.WriteError)
			handler := FileTransfer(config.Limits{
				Enabled:      tc.LimitsEnabled,
				FileTransfer: tc.Limits,
			})().(*FileTransferHandler)
			b, _ := msgpack.Marshal(tc.Params)
			request := &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypePut,
					SessionID: "12344",
				},
				Body: b,
			}
			defer handler.Close()

			handler.ServeProtoMsg(request, recorder)
			select {
			case <-recorder.Called:
			case <-time.After(time.Second * 10):
				panic("test case timeout")
			}

			if tc.TransferMessages != nil {
				for _, msg := range tc.TransferMessages {
					handler.ServeProtoMsg(msg, recorder)
				}
			}

			if tc.FileContents != nil {
				c := int64(tc.ChunkSize)
				l := int64(len(tc.FileContents))
				for start := int64(0); start < l; start += c {
					end := start + c
					if end > l {
						end = l
					}
					handler.ServeProtoMsg(&ws.ProtoMsg{
						Header: ws.ProtoHdr{
							Proto:     ws.ProtoTypeFileTransfer,
							MsgType:   wsft.MessageTypeChunk,
							SessionID: "12344",
							Properties: map[string]interface{}{
								"offset": start,
							},
						},
						Body: tc.FileContents[start:end],
					}, recorder)

				}
				handler.ServeProtoMsg(&ws.ProtoMsg{
					Header: ws.ProtoHdr{
						Proto:     ws.ProtoTypeFileTransfer,
						MsgType:   wsft.MessageTypeChunk,
						SessionID: "12344",
						Properties: map[string]interface{}{
							"offset": l,
						},
					},
				}, recorder)
			}
			select {
			case handler.mutex <- struct{}{}:

			case <-handler.mutex:
				handler.mutex <- struct{}{}
				// Block until the handler finishes
				handler.mutex <- struct{}{}
			}

			if !assert.GreaterOrEqual(t, len(recorder.Messages), 1) {
				t.FailNow()
			}
			if tc.Error == nil {
				for _, msg := range recorder.Messages {
					pass := assert.Equal(
						t, wsft.MessageTypeACK, msg.Header.MsgType,
						"Bad message: %v", msg,
					)
					if assert.Contains(t, msg.Header.Properties, "offset") {
						pass = pass && assert.LessOrEqual(t,
							msg.Header.Properties["offset"].(int64),
							int64(len(tc.FileContents)),
						)
					} else {
						pass = false
					}
					if !pass {
						t.FailNow()
					}
				}
				lastAck := recorder.Messages[len(recorder.Messages)-1]
				if assert.Contains(t, lastAck.Header.Properties, "offset") {
					assert.Equal(t,
						int64(len(tc.FileContents)),
						lastAck.Header.Properties["offset"].(int64),
					)
				}
			} else {
				if tc.WriteError != nil {
					assert.Len(t, recorder.Messages, 1)
					return
				}
				errMsgs := []*ws.ProtoMsg{}
				for _, msg := range recorder.Messages {
					if msg.Header.MsgType == wsft.MessageTypeError {
						errMsgs = append(errMsgs, msg)
					}
				}
				if !assert.GreaterOrEqual(t, len(errMsgs), 1, "did not receive any errors") {
					t.FailNow()
				}
				lastErr := errMsgs[len(errMsgs)-1]
				if !assert.Equal(t,
					wsft.MessageTypeError,
					lastErr.Header.MsgType,
				) {
					t.FailNow()
				}
				var err wsft.Error
				msgpack.Unmarshal(lastErr.Body, &err)
				if assert.NotNil(t, err.Error) {
					assert.Contains(t, *err.Error, tc.Error.Error())
				}
			}
		})
	}
}

type ChanWriter struct {
	C chan *ws.ProtoMsg
}

func NewChanWriter(cap int) *ChanWriter {
	return &ChanWriter{
		C: make(chan *ws.ProtoMsg, cap),
	}
}

func (w ChanWriter) WriteProtoMsg(msg *ws.ProtoMsg) error {
	msgCP := &ws.ProtoMsg{
		Header: msg.Header,
	}
	if msg.Body != nil {
		msgCP.Body = make([]byte, len(msg.Body))
		copy(msgCP.Body, msg.Body)
	}
	w.C <- msgCP
	return nil
}

func TestFileTransferDownload(t *testing.T) {
	t.Parallel()
	testdir, err := ioutil.TempDir("", "filetransferTesting")
	if err != nil {
		panic(err)
	}
	t.Cleanup(func() { os.RemoveAll(testdir) })

	testCases := []struct {
		Name string

		FileContents []byte
		Acker        func(msg *ws.ProtoMsg) *ws.ProtoMsg

		LimitsEnabled    bool
		Limits           config.FileTransferLimits
		FileDoesNotExist bool

		Error error
	}{{
		Name: "ok",

		FileContents: []byte("a small chunk of bytes..."),
		Acker: func(msg *ws.ProtoMsg) *ws.ProtoMsg {
			ret := &ws.ProtoMsg{
				Header: msg.Header,
			}
			ret.Header.MsgType = wsft.MessageTypeACK
			return ret
		},
	}, {
		Name: "ok, file larger than window",

		FileContents: func() []byte {
			b := make([]byte, FileTransferBufSize*(ACKSlidingWindowRecv+1))
			copy(b, "zeroes...")
			return b
		}(),
		Acker: func(msg *ws.ProtoMsg) *ws.ProtoMsg {
			ret := &ws.ProtoMsg{
				Header: msg.Header,
			}
			ret.Header.MsgType = wsft.MessageTypeACK
			return ret
		},
	}, {
		Name: "error, bad ack data type",

		FileContents: []byte("tiny chunk"),

		Acker: func(msg *ws.ProtoMsg) *ws.ProtoMsg {
			if msg.Body != nil {
				return nil
			}
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeACK,
					Properties: map[string]interface{}{
						"offset": "12",
					},
				},
			}
		},
		Error: errors.New("invalid offset data type: require int64"),
	}, {
		Name: "error, no offset in ack",

		FileContents: []byte("tiny chunk"),

		Acker: func(msg *ws.ProtoMsg) *ws.ProtoMsg {
			if msg.Body != nil {
				return nil
			}
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeACK,
				},
			}
		},
		Error: errors.New("ack message: offset property cannot be blank"),
	}, {
		Name: "error, unexpected ack message",

		FileContents: []byte("tiny chunk"),

		Acker: func(msg *ws.ProtoMsg) *ws.ProtoMsg {
			if msg.Body != nil {
				return nil
			}
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeChunk,
				},
				Body: []byte("I am violating the protocol haha"),
			}
		},
		Error: errors.New("received unexpected message type 'file_chunk'; expected 'ack'"),
	}, {
		Name: "error, client malformed error message",

		FileContents: []byte("tiny chunk"),

		Acker: func(msg *ws.ProtoMsg) *ws.ProtoMsg {
			if msg.Body != nil {
				return nil
			}
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeError,
				},
				Body: []byte("bad error schema"),
			}
		},
		// The error will abort the handler s.t. no message is returned.
	},
		{
			Name: "error, client error message",

			FileContents: []byte("tiny chunk"),

			Acker: func(msg *ws.ProtoMsg) *ws.ProtoMsg {
				if msg.Body != nil {
					return nil
				}
				errMsg := "ENOSPC"
				b, _ := msgpack.Marshal(wsft.Error{
					Error: &errMsg,
				})
				return &ws.ProtoMsg{
					Header: ws.ProtoHdr{
						Proto:   ws.ProtoTypeFileTransfer,
						MsgType: wsft.MessageTypeError,
					},
					Body: b,
				}
			},
			// The error will abort the handler s.t. no message is returned.
		},
		{
			Name: "error, file does not exist",

			FileContents: []byte("tiny chunk"),
			Acker: func(msg *ws.ProtoMsg) *ws.ProtoMsg {
				ret := &ws.ProtoMsg{
					Header: msg.Header,
				}
				ret.Header.MsgType = wsft.MessageTypeACK
				return ret
			},
			FileDoesNotExist: true,

			Error: errors.New("failed to open file for reading: open lets say it does not exist: no such file or directory"),
		},
		{
			Name: "error, forbidden to follow links",

			FileContents: []byte("tiny chunk"),
			Acker: func(msg *ws.ProtoMsg) *ws.ProtoMsg {
				ret := &ws.ProtoMsg{
					Header: msg.Header,
				}
				ret.Header.MsgType = wsft.MessageTypeACK
				return ret
			},

			LimitsEnabled: true,
			Limits: config.FileTransferLimits{
				FollowSymLinks: false,
			},

			Error: filetransfer.ErrFollowLinksForbidden,
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			w := NewChanWriter(ACKSlidingWindowRecv)
			handler := FileTransfer(config.Limits{
				Enabled:      tc.LimitsEnabled,
				FileTransfer: tc.Limits,
			})().(*FileTransferHandler)
			fd, err := ioutil.TempFile(testdir, "testfile")
			if err != nil {
				panic(err)
			}
			filename := fd.Name()
			n, err := fd.Write(tc.FileContents)
			fd.Close()
			if err != nil {
				panic(err)
			}
			assert.Equal(t, len(tc.FileContents), n)
			if tc.LimitsEnabled {
				if !tc.Limits.FollowSymLinks {
					err := os.Symlink(filename, filename+"-link")
					if err != nil {
						t.Fatal("cant create a link")
					}
					filename = filename + "-link"
				}
			}
			if tc.FileDoesNotExist {
				filename = "lets say it does not exist"
			}

			b, _ := msgpack.Marshal(wsft.GetFile{
				Path: &filename,
			})
			request := &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeGet,
				},
				Body: b,
			}
			defer handler.Close()

			recvBuf := bytes.NewBuffer(nil)
			handler.ServeProtoMsg(request, w)
			timeout := time.NewTimer(time.Second * 10)
			var msg *ws.ProtoMsg
			select {
			case msg = <-w.C:
				timeout.Reset(time.Second * 10)
			case <-timeout.C:
				panic("test case timeout")
			}
			var offset int64
		Loop:
			for {
				if msg.Header.MsgType == wsft.MessageTypeChunk {
					off := msg.Header.Properties["offset"].(int64)
					assert.GreaterOrEqual(t, off, offset)
					offset = off
					recvBuf.Write(msg.Body)
					rsp := tc.Acker(msg)
					if rsp != nil {
						handler.ServeProtoMsg(rsp, w)
					}
				} else {
					break Loop
				}
				select {
				case handler.mutex <- struct{}{}:
					break Loop
				case msg = <-w.C:

				case <-timeout.C:
					panic("test case timeout")
				}
			}

			if tc.Error != nil {
				if !assert.Equal(t,
					wsft.MessageTypeError,
					msg.Header.MsgType,
				) {
					t.FailNow()
				}
				var erro wsft.Error
				msgpack.Unmarshal(msg.Body, &erro)
				if assert.NotNil(t, erro.Error) {
					assert.Contains(t,
						*erro.Error,
						tc.Error.Error(),
					)
				}
			} else {
				// Last message must be an EOF and the file contents
				// must match.
				if !assert.Equal(t,
					wsft.MessageTypeChunk,
					msg.Header.MsgType,
				) {
					t.FailNow()
				}
				assert.Nil(t, msg.Body, "Last message must be an EOF chunk")
				assert.Equal(t, tc.FileContents, recvBuf.Bytes())
			}
		})
	}
}

func TestFileTransferStat(t *testing.T) {
	t.Parallel()
	const (
		Filecontents = "test data"
	)
	var filename string
	fd, err := ioutil.TempFile("", "test_filetransfer")
	if err != nil {
		panic(err)
	}
	filename = fd.Name()
	t.Cleanup(func() {
		fd.Close()
		os.Remove(filename)
	})
	_, err = fd.Write([]byte(Filecontents))
	if err != nil {
		panic(err)
	}
	testCases := []struct {
		Name string

		Message    *ws.ProtoMsg
		WriteError error

		ResponseValidator func(*testing.T, []*ws.ProtoMsg)
	}{{
		Name: "ok",

		Message: func() *ws.ProtoMsg {
			b, _ := msgpack.Marshal(wsft.StatFile{
				Path: &filename,
			})
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeStat,
				},
				Body: b,
			}
		}(),
		ResponseValidator: func(t *testing.T, msgs []*ws.ProtoMsg) {
			if assert.Len(t, msgs, 1) {
				assert.Equal(t, wsft.MessageTypeFileInfo, msgs[0].Header.MsgType)
				var fileInfo wsft.FileInfo
				msgpack.Unmarshal(msgs[0].Body, &fileInfo)
				if assert.NotNil(t, fileInfo.Path) {
					assert.Equal(t, filename, *fileInfo.Path)
				}
				assert.NotNil(t, fileInfo.UID)
				assert.NotNil(t, fileInfo.GID)
				if assert.NotNil(t, fileInfo.ModTime) {
					assert.WithinDuration(t,
						time.Now(),
						*fileInfo.ModTime,
						time.Minute,
					)
				}
				assert.NotNil(t, fileInfo.ModTime)
				assert.NotNil(t, fileInfo.Mode)
			}
		},
	}, {
		Name: "error, malformed schema",

		Message: func() *ws.ProtoMsg {
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeStat,
				},
				Body: []byte("foobar"),
			}
		}(),
		ResponseValidator: func(t *testing.T, msgs []*ws.ProtoMsg) {
			if assert.Len(t, msgs, 1) {
				assert.Equal(t, wsft.MessageTypeError, msgs[0].Header.MsgType)
				var erro wsft.Error
				msgpack.Unmarshal(msgs[0].Body, &erro)
				if assert.NotNil(t, erro.Error) {
					assert.Contains(t,
						*erro.Error,
						"malformed request parameters",
					)
				}
			}
		},
	}, {
		Name: "error, invalid parameters",

		Message: func() *ws.ProtoMsg {
			b, _ := msgpack.Marshal(map[string]interface{}{
				"path": nil,
			})
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeStat,
				},
				Body: b,
			}
		}(),
		ResponseValidator: func(t *testing.T, msgs []*ws.ProtoMsg) {
			if assert.Len(t, msgs, 1) {
				assert.Equal(t, wsft.MessageTypeError, msgs[0].Header.MsgType)
				var erro wsft.Error
				msgpack.Unmarshal(msgs[0].Body, &erro)
				if assert.NotNil(t, erro.Error) {
					assert.Contains(t,
						*erro.Error,
						"invalid request parameters: path: "+
							"cannot be blank.",
					)
				}
			}
		},
	}, {
		Name: "error, invalid parameters",

		Message: func() *ws.ProtoMsg {
			noexist := path.Join(filename, "does", "not", "exist")
			b, _ := msgpack.Marshal(wsft.StatFile{
				Path: &noexist,
			})
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeStat,
				},
				Body: b,
			}
		}(),
		ResponseValidator: func(t *testing.T, msgs []*ws.ProtoMsg) {
			if assert.Len(t, msgs, 1) {
				assert.Equal(t, wsft.MessageTypeError, msgs[0].Header.MsgType)
				var erro wsft.Error
				msgpack.Unmarshal(msgs[0].Body, &erro)
				if assert.NotNil(t, erro.Error) {
					assert.Contains(t,
						*erro.Error,
						"failed to get file info from path",
					)
				}
			}
		},
	}}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			handler := FileTransfer(config.Limits{})()
			w := NewTestWriter(tc.WriteError)
			handler.ServeProtoMsg(tc.Message, w)
			tc.ResponseValidator(t, w.Messages)
		})
	}
}

func TestFileTransferServeErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name string

		Message   *ws.ProtoMsg
		LockMutex bool

		Error error
	}{{
		Name: "malformed upload request",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypePut,
				SessionID: "1234",
			},
			Body: []byte("/foo/bar"),
		},

		Error: errors.New("malformed request parameters: "),
	}, {
		Name: "invalid upload request parameters",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypePut,
				SessionID: "1234",
			},
			Body: func() []byte {
				b, _ := msgpack.Marshal(map[string]interface{}{})
				return b
			}(),
		},

		Error: errors.New("invalid request parameters: path: cannot be blank"),
	}, {
		Name: "upload already in progress",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypePut,
				SessionID: "1234",
			},
			Body: func() []byte {
				fd, err := ioutil.TempFile("", "filetransferTesting")
				if err != nil {
					panic(err)
				}
				filename := fd.Name()
				fd.Close()
				t.Cleanup(func() { os.Remove(fd.Name()) })
				b, _ := msgpack.Marshal(wsft.FileInfo{
					Path: &filename,
				})
				return b
			}(),
		},
		LockMutex: true,

		Error: errors.New("another file transfer is in progress"),
	}, {
		Name: "malformed download request",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypeGet,
				SessionID: "1234",
			},
			Body: []byte("/foo/bar"),
		},

		Error: errors.New("malformed request parameters: "),
	}, {
		Name: "invalid download request parameters",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypeGet,
				SessionID: "1234",
			},
			Body: func() []byte {
				b, _ := msgpack.Marshal(map[string]interface{}{})
				return b
			}(),
		},

		Error: errors.New("invalid request parameters: path: cannot be blank"),
	}, {
		Name: "download already in progress",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypeGet,
				SessionID: "1234",
			},
			Body: func() []byte {
				fd, err := ioutil.TempFile("", "filetransferTesting")
				if err != nil {
					panic(err)
				}
				filename := fd.Name()
				fd.Close()
				t.Cleanup(func() { os.Remove(fd.Name()) })
				b, _ := msgpack.Marshal(wsft.GetFile{
					Path: &filename,
				})
				return b
			}(),
		},
		LockMutex: true,

		Error: errors.New("another file transfer is in progress"),
	}, {
		Name: "got chunk but no file transfer in progress",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypeChunk,
				SessionID: "1234",
			},
		},

		Error: errors.New("no file transfer in progress"),
	}, {
		Name: "generic error from client",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypeError,
				SessionID: "1234",
			},
			Body: func() []byte {
				errMsg := "generic error"
				b, _ := msgpack.Marshal(wsft.Error{
					Error: &errMsg,
				})
				return b
			}(),
		},
	}, {
		Name: "malformed error from client",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypeError,
				SessionID: "1234",
			},
			Body: []byte("schemaless error?"),
		},
	}, {
		Name: "message type not implemented",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   "SpecialSauce/NotImplemented",
				SessionID: "1234",
			},
		},

		Error: errors.New("session: filetransfer message type " +
			"'SpecialSauce/NotImplemented' not supported"),
	}}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			handler := FileTransfer(config.Limits{})().(*FileTransferHandler)
			if tc.LockMutex {
				handler.mutex <- struct{}{}
			}
			w := NewTestWriter(nil)
			handler.ServeProtoMsg(tc.Message, w)

			if tc.Error != nil {
				if !assert.Len(t, w.Messages, 1) {
					t.FailNow()
				}
				msg := w.Messages[0]
				assert.Equal(t, wsft.MessageTypeError, msg.Header.MsgType)
				var erro wsft.Error
				msgpack.Unmarshal(msg.Body, &erro)
				if assert.NotNil(t, erro.Error) {
					assert.Contains(t, *erro.Error, tc.Error.Error())
				}
			} else {
				assert.Len(t, w.Messages, 0)
			}
		})
	}
}
