package session

import (
	"io"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack"

	"github.com/mendersoftware/go-lib-micro/ws"
	wsft "github.com/mendersoftware/go-lib-micro/ws/filetransfer"
)

type FileTransferHandler struct {
	file    *os.File
	filePos int64
}

func FileTransfer() Constructor {
	return func() SessionHandler { return new(FileTransferHandler) }
}

func (h *FileTransferHandler) Error(msg *ws.ProtoMsg, w ResponseWriter, err error) {
	errMsg := err.Error()
	msgErr := wsft.Error{
		Error:       &errMsg,
		MessageType: &msg.Header.MsgType,
	}
	rsp := *msg
	rsp.Header.MsgType = wsft.MessageTypeError
	rsp.Body, _ = msgpack.Marshal(msgErr)
	w.WriteProtoMsg(&rsp) //nolint:errcheck
}

func (h *FileTransferHandler) Close() error {
	if h.file != nil {
		err := errors.New("session: filetransfer closed unexpectedly")
		filename := h.file.Name()
		h.file.Close() //nolint:errcheck
		errRm := os.Remove(filename)
		if errRm != nil {
			err = errors.Wrapf(err,
				"failed to remove incomplete file '%s': %s",
				filename, errRm.Error(),
			)
		}
		h.file = nil
		h.filePos = 0
		return err
	}
	return nil
}

func (h *FileTransferHandler) ServeProtoMsg(msg *ws.ProtoMsg, w ResponseWriter) {
	switch msg.Header.MsgType {
	case wsft.MessageTypePut:
		if h.file != nil {
			h.Error(msg, w, errors.New("session: file transfer already in progress"))
			return
		}
		var (
			defaultMode uint32 = 0644
			defaultUID  uint32 = uint32(os.Getuid())
			defaultGID  uint32 = uint32(os.Getgid())
		)
		params := wsft.FileInfo{
			UID:  &defaultUID,
			GID:  &defaultGID,
			Mode: &defaultMode,
		}
		err := msgpack.Unmarshal(msg.Body, &params)
		if err != nil {
			h.Error(msg, w, errors.Wrap(err, "session: malformed request body"))
			return
		} else if params.Path == nil {
			h.Error(msg, w, errors.New(
				"session: invalid request parameters: path cannot be empty",
			))
			return
		}
		err = h.InitFileUpload(params)
		if err != nil {
			h.Error(msg, w, err)
			return
		}
		rsp := *msg
		rsp.Header.MsgType = wsft.MessageTypeContinue
		rsp.Body = nil
		err = w.WriteProtoMsg(&rsp)
		if err != nil {
			log.Error("session: failed to respond to client")
		}

	case wsft.MessageTypeChunk:
		if h.file == nil {
			h.Error(msg, w, errors.New("session: no file transfer in progress"))
			return
		}
		offset, ok := msg.Header.Properties["offset"].(int64)
		if !ok {
			// Append to file
			offset = int64(-1)
		}
		if len(msg.Body) > 0 {
			err := h.WriteOffset(msg.Body, offset)
			if err != nil {
				h.Error(msg, w, err)
			}
		} else {
			// EOF
			err := h.file.Close()
			if err != nil {
				h.Error(msg, w, errors.Wrap(err, "error completing file transfer"))
				return
			}
			h.file = nil
			h.filePos = 0
		}

	case wsft.MessageTypeStat:
		// TODO

	case wsft.MessageTypeGet:
		// TODO

	default:
		h.Error(msg, w, errors.Errorf(
			"session: protocol violation: unexpected filetransfer message type: %s",
			msg.Header.MsgType,
		))
	}
}

func (h *FileTransferHandler) WriteOffset(b []byte, offset int64) (err error) {
	defer func() {
		if err != nil {
			filename := h.file.Name()
			h.file.Close() //nolint:errcheck
			h.file = nil
			h.filePos = 0
			errRm := os.Remove(filename)
			if errRm != nil {
				log.Warn("failed to remove file after failed write: " +
					err.Error())
			}
		}
	}()
	if offset >= 0 {
		if h.filePos != offset {
			h.filePos, err = h.file.Seek(offset, io.SeekStart)
			if err != nil {
				return errors.Wrap(err, "session: failed to seek to file offset")
			} else if h.filePos != offset {
				return errors.New("session: failed to seek to file offset")
			}
		}
	}
	n, err := h.file.Write(b)
	h.filePos += int64(n)
	if err != nil {
		return errors.Wrap(err, "session: failed to write file chunk")
	}
	return nil
}

func (h *FileTransferHandler) InitFileUpload(params wsft.FileInfo) error {
	fd, err := os.OpenFile(
		*params.Path,
		os.O_CREATE|os.O_WRONLY,
		os.FileMode(*params.Mode),
	)
	if err != nil {
		return errors.Wrap(err, "session: failed to create file")
	}
	err = fd.Chown(int(*params.UID), int(*params.GID))
	if err != nil {
		return errors.Wrap(err, "session: failed to set file permissions")
	}
	h.file = fd
	return nil
}
