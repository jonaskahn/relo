// Event-stream decoding: length-prefixed message frames.
package eventstream

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

// Event-stream errors name why a Bedrock frame cannot be trusted: a checksum
// mismatch or a message too short to hold one.
var (
	ErrPreludeCRCMismatch = errors.New("eventstream: prelude CRC mismatch")
	ErrMessageCRCMismatch = errors.New("eventstream: message CRC mismatch")
	ErrFrameTooSmall      = errors.New("eventstream: frame too small")
)

const (
	minFrameLen   = 16
	headerTypeStr = 7
)

var crcTable = crc32.MakeTable(crc32.IEEE)

// Message is one parsed eventstream frame.
type Message struct {
	Headers map[string]string
	Payload []byte
}

// Decoder reads eventstream binary frames from a stream.
type Decoder struct {
	reader io.Reader
}

// NewDecoder creates a new eventstream Decoder.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{reader: r}
}

// DecodeNext decodes the next message from the stream.
func (d *Decoder) DecodeNext() (Message, error) {
	prelude, err := d.readPrelude()
	if err != nil {
		return Message{}, err
	}

	totalLen := binary.BigEndian.Uint32(prelude[0:4])
	headersLen := binary.BigEndian.Uint32(prelude[4:8])
	if totalLen < minFrameLen || totalLen < 12+headersLen+4 {
		return Message{}, ErrFrameTooSmall
	}

	rest, err := d.readFrameRest(totalLen)
	if err != nil {
		return Message{}, err
	}

	msgBytes := append(prelude[:], rest...)
	if err := verifyFrameCRC(msgBytes); err != nil {
		return Message{}, err
	}

	headersBytes := rest[:headersLen]
	payloadBytes := rest[headersLen : len(rest)-4]

	return Message{
		Headers: parseHeaders(headersBytes),
		Payload: payloadBytes,
	}, nil
}

func verifyFrameCRC(msgBytes []byte) error {
	msgCRC := binary.BigEndian.Uint32(msgBytes[len(msgBytes)-4:])
	expectedMsgCRC := crc32.Checksum(msgBytes[:len(msgBytes)-4], crcTable)
	if msgCRC != expectedMsgCRC {
		return ErrMessageCRCMismatch
	}
	return nil
}

func (d *Decoder) readFrameRest(totalLen uint32) ([]byte, error) {
	restLen := totalLen - 12
	rest := make([]byte, restLen)
	if _, err := io.ReadFull(d.reader, rest); err != nil {
		return nil, err
	}
	return rest, nil
}

func (d *Decoder) readPrelude() ([12]byte, error) {
	var prelude [12]byte
	if _, err := io.ReadFull(d.reader, prelude[:]); err != nil {
		return prelude, err
	}

	preludeCRC := binary.BigEndian.Uint32(prelude[8:12])
	expectedPreludeCRC := crc32.Checksum(prelude[0:8], crcTable)
	if preludeCRC != expectedPreludeCRC {
		return prelude, ErrPreludeCRCMismatch
	}
	return prelude, nil
}

func parseHeaders(data []byte) map[string]string {
	headers := make(map[string]string)
	buf := bytes.NewReader(data)

	for buf.Len() > 0 {
		name, value, ok := parseHeader(buf)
		if !ok {
			break
		}
		headers[name] = value
	}
	return headers
}

func parseHeader(buf *bytes.Reader) (string, string, bool) {
	nameLen, err := buf.ReadByte()
	if err != nil {
		return "", "", false
	}
	nameBytes := make([]byte, nameLen)
	if _, err := io.ReadFull(buf, nameBytes); err != nil {
		return "", "", false
	}
	valType, err := buf.ReadByte()
	if err != nil {
		return "", "", false
	}
	if valType != headerTypeStr {
		return "", "", false
	}
	var valLen uint16
	if err := binary.Read(buf, binary.BigEndian, &valLen); err != nil {
		return "", "", false
	}
	valBytes := make([]byte, valLen)
	if _, err := io.ReadFull(buf, valBytes); err != nil {
		return "", "", false
	}
	return string(nameBytes), string(valBytes), true
}

// EncodeFrame encodes a Message into binary eventstream frame.
func EncodeFrame(headers map[string]string, payload []byte) []byte {
	var headersBuf bytes.Buffer
	for k, v := range headers {
		headersBuf.WriteByte(byte(len(k)))
		headersBuf.WriteString(k)
		headersBuf.WriteByte(headerTypeStr)
		var valLen = uint16(len(v))
		_ = binary.Write(&headersBuf, binary.BigEndian, valLen)
		headersBuf.WriteString(v)
	}

	headersBytes := headersBuf.Bytes()
	totalLen := 12 + len(headersBytes) + len(payload) + 4

	var frame bytes.Buffer
	_ = binary.Write(&frame, binary.BigEndian, uint32(totalLen))
	_ = binary.Write(&frame, binary.BigEndian, uint32(len(headersBytes)))

	preludeCRC := crc32.Checksum(frame.Bytes(), crcTable)
	_ = binary.Write(&frame, binary.BigEndian, preludeCRC)

	frame.Write(headersBytes)
	frame.Write(payload)

	msgCRC := crc32.Checksum(frame.Bytes(), crcTable)
	_ = binary.Write(&frame, binary.BigEndian, msgCRC)

	return frame.Bytes()
}
