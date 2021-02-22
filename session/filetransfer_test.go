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
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/mendersoftware/go-lib-micro/ws"
	wsft "github.com/mendersoftware/go-lib-micro/ws/filetransfer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack"
)

func TestFileTransferPutFile(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name string

		Filename   string
		Messages   func(filename string) []ws.ProtoMsg
		WriteError error
		CloseError error

		ResponseValidator func(*testing.T, []*ws.ProtoMsg)
		Filecontents      []byte
	}{{
		Name: "ok",

		Messages: func(filename string) []ws.ProtoMsg {
			b, _ := msgpack.Marshal(wsft.FileInfo{
				Path: &filename,
			})
			return []ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypePut,
				},
				Body: b,
			}, {
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeChunk,
				},
				Body: []byte("chunk1"),
			}, {
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeChunk,
					Properties: map[string]interface{}{
						"offset": int64(len("chunk1") - 1),
					},
				},
				Body: []byte("chunk2"),
			}, {
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeChunk,
				},
			}}
		},
		ResponseValidator: func(t *testing.T, rsp []*ws.ProtoMsg) {
			assert.Equal(t, rsp, []*ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeContinue,
				},
			}})
		},
		Filecontents: []byte("chunkchunk2"),
	}, {
		Name: "error, upload didn't complete",

		Messages: func(filename string) []ws.ProtoMsg {
			b, _ := msgpack.Marshal(wsft.FileInfo{
				Path: &filename,
			})
			return []ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypePut,
				},
				Body: b,
			}, {
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeChunk,
				},
				Body: []byte("chunk1"),
			}}
		},
		ResponseValidator: func(t *testing.T, rsp []*ws.ProtoMsg) {
			assert.Equal(t, rsp, []*ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeContinue,
				},
			}})
		},
		CloseError:   errors.New("session: filetransfer closed unexpectedly"),
		Filecontents: []byte("chunk1"),
	}, {
		Name: "error, malformed schema",

		Messages: func(filename string) []ws.ProtoMsg {
			return []ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypePut,
				},
				Body: []byte("foobar"),
			}}
		},
		ResponseValidator: func(t *testing.T, rsp []*ws.ProtoMsg) {
			msg := "session: malformed request body: msgpack: " +
				"invalid code=66 decoding map length"
			msgtyp := wsft.MessageTypePut
			b, _ := msgpack.Marshal(wsft.Error{
				Error:       &msg,
				MessageType: &msgtyp,
			})
			assert.Equal(t, []*ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeError,
				},
				Body: b,
			}}, rsp)
		},
	}, {
		Name: "error, invalid parameters",

		Messages: func(filename string) []ws.ProtoMsg {
			uid := uint32(0)
			b, _ := msgpack.Marshal(wsft.FileInfo{UID: &uid})
			return []ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypePut,
				},
				Body: b,
			}}
		},
		ResponseValidator: func(t *testing.T, rsp []*ws.ProtoMsg) {
			msg := "session: invalid request parameters: path: " +
				"cannot be blank."
			msgtyp := wsft.MessageTypePut
			b, _ := msgpack.Marshal(wsft.Error{
				Error:       &msg,
				MessageType: &msgtyp,
			})
			assert.Equal(t, []*ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypeError,
				},
				Body: b,
			}}, rsp)
		},
	}, {
		Name: "error, from writer",

		Messages: func(filename string) []ws.ProtoMsg {
			b, _ := msgpack.Marshal(wsft.FileInfo{Path: &filename})
			return []ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypePut,
				},
				Body: b,
			}}
		},
		WriteError: errors.New("bad writer"),
		ResponseValidator: func(t *testing.T, rsp []*ws.ProtoMsg) {
			assert.Len(t, rsp, 1)
		},
		CloseError: errors.New("session: filetransfer closed unexpectedly"),
	}, {
		Name: "error, transfer already in progress",

		Messages: func(filename string) []ws.ProtoMsg {
			b, _ := msgpack.Marshal(wsft.FileInfo{Path: &filename})
			return []ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypePut,
				},
				Body: b,
			}, {
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypePut,
				},
				Body: b,
			}}
		},
		ResponseValidator: func(t *testing.T, rsp []*ws.ProtoMsg) {
			if assert.Len(t, rsp, 2) {
				var erro wsft.Error
				msgpack.Unmarshal(rsp[1].Body, &erro)
				msg := "session: file upload already in progress"
				msgtyp := wsft.MessageTypePut
				assert.Equal(t, wsft.Error{
					Error:       &msg,
					MessageType: &msgtyp,
				}, erro)
			}
		},
		CloseError: errors.New("session: filetransfer closed unexpectedly"),
	}, {
		Name: "error, parent directory does not exist",

		Messages: func(filename string) []ws.ProtoMsg {
			noexist := path.Join(filename, "not", "exist")
			b, _ := msgpack.Marshal(wsft.FileInfo{Path: &noexist})
			return []ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypePut,
				},
				Body: b,
			}, {
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: wsft.MessageTypePut,
				},
				Body: b,
			}}
		},
		ResponseValidator: func(t *testing.T, rsp []*ws.ProtoMsg) {
			if assert.Len(t, rsp, 2) {
				var erro wsft.Error
				msgpack.Unmarshal(rsp[1].Body, &erro)
				assert.Contains(t, *erro.Error, "session: failed to create file")
			}
		},
	}, {
		Name: "error, parent directory does not exist",

		Messages: func(filename string) []ws.ProtoMsg {
			return []ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:   ws.ProtoTypeFileTransfer,
					MsgType: "foobar",
				},
			}}
		},
		ResponseValidator: func(t *testing.T, rsp []*ws.ProtoMsg) {
			if assert.Len(t, rsp, 1) {
				var erro wsft.Error
				err := msgpack.Unmarshal(rsp[0].Body, &erro)
				if assert.NoError(t, err) {
					assert.Contains(t,
						*erro.Error,
						"session: filetransfer message "+
							"type 'foobar' not supported",
					)
				}
			}
		},
	}}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			handler := FileTransfer()()
			f, err := ioutil.TempFile("", "test_filetransfer")
			if err != nil {
				panic(err)
			}
			filename := f.Name()
			f.Close()
			os.Remove(filename)
			w := &testWriter{err: tc.WriteError}
			messages := tc.Messages(f.Name())
			for i := range messages {
				handler.ServeProtoMsg(&messages[i], w)
			}
			tc.ResponseValidator(t, w.Messages)
			if tc.Filecontents != nil {
				contents, err := ioutil.ReadFile(filename)
				assert.NoError(t, err)
				assert.Equal(t, string(tc.Filecontents), string(contents))
			}
			err = handler.Close()
			if tc.CloseError != nil {
				assert.EqualError(t, err, tc.CloseError.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFileTransferGetFile(t *testing.T) {
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

		Responses []*ws.ProtoMsg
	}{{
		Name: "ok",

		Message: func() *ws.ProtoMsg {
			b, _ := msgpack.Marshal(wsft.GetFile{
				Path: &filename,
			})
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeGet,
					SessionID: "1234",
				},
				Body: b,
			}
		}(),
		Responses: []*ws.ProtoMsg{{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypeChunk,
				SessionID: "1234",
				Properties: map[string]interface{}{
					"offset": int64(0),
				},
			},
			Body: []byte(Filecontents),
		}, {
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypeChunk,
				SessionID: "1234",
				Properties: map[string]interface{}{
					"offset": int64(len(Filecontents)),
				},
			},
		}},
	}, {
		Name: "error, bad writer",

		Message: func() *ws.ProtoMsg {
			b, _ := msgpack.Marshal(wsft.GetFile{
				Path: &filename,
			})
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeGet,
					SessionID: "1234",
				},
				Body: b,
			}
		}(),
		WriteError: errors.New("bad writer"),
		Responses: func() []*ws.ProtoMsg {
			msg := "failed to write file chunk to stream: abort: bad writer"
			msgtyp := wsft.MessageTypeGet
			erro := wsft.Error{
				Error:       &msg,
				MessageType: &msgtyp,
			}
			b, _ := msgpack.Marshal(erro)
			return []*ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeChunk,
					SessionID: "1234",
					Properties: map[string]interface{}{
						"offset": int64(0),
					},
				},
				Body: []byte(Filecontents),
			}, {
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeError,
					SessionID: "1234",
				},
				Body: b,
			}}
		}(),
	}, {
		Name: "error, file does not exist",

		Message: func() *ws.ProtoMsg {
			filename := "/does/not/exist/1234"
			b, _ := msgpack.Marshal(wsft.GetFile{
				Path: &filename,
			})
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeGet,
					SessionID: "1234",
				},
				Body: b,
			}
		}(),
		Responses: func() []*ws.ProtoMsg {
			msg := "failed to open file for reading: " +
				"open /does/not/exist/1234: no such file or directory"
			msgtyp := wsft.MessageTypeGet
			erro := wsft.Error{
				Error:       &msg,
				MessageType: &msgtyp,
			}
			b, _ := msgpack.Marshal(erro)
			return []*ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeError,
					SessionID: "1234",
				},
				Body: b,
			}}
		}(),
	}, {
		Name: "error, malformed request parameters",

		Message: &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   wsft.MessageTypeGet,
				SessionID: "1234",
			},
			Body: []byte("foobar"),
		},
		Responses: func() []*ws.ProtoMsg {
			msg := "malformed request parameters: msgpack: " +
				"invalid code=66 decoding map length"
			msgtyp := wsft.MessageTypeGet
			erro := wsft.Error{
				Error:       &msg,
				MessageType: &msgtyp,
			}
			b, _ := msgpack.Marshal(erro)
			return []*ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeError,
					SessionID: "1234",
				},
				Body: b,
			}}
		}(),
	}, {
		Name: "error, invalid request parameters",

		Message: func() *ws.ProtoMsg {
			b, _ := msgpack.Marshal(GetFile{})
			return &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeGet,
					SessionID: "1234",
				},
				Body: b,
			}
		}(),
		Responses: func() []*ws.ProtoMsg {
			msg := "invalid request parameters: path: cannot be blank."
			msgtyp := wsft.MessageTypeGet
			erro := wsft.Error{
				Error:       &msg,
				MessageType: &msgtyp,
			}
			b, _ := msgpack.Marshal(erro)
			return []*ws.ProtoMsg{{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypeFileTransfer,
					MsgType:   wsft.MessageTypeError,
					SessionID: "1234",
				},
				Body: b,
			}}
		}(),
	}}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			handler := FileTransfer()()
			w := &testWriter{err: tc.WriteError}
			handler.ServeProtoMsg(tc.Message, w)
			time.Sleep(time.Second)
			assert.Equal(t, tc.Responses, w.Messages)
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

			handler := FileTransfer()()
			w := &testWriter{err: tc.WriteError}
			handler.ServeProtoMsg(tc.Message, w)
			tc.ResponseValidator(t, w.Messages)
		})
	}
}
