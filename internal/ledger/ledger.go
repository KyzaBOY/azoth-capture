// Package ledger reads and splits ledger.bin, the DTLS record file the
// capture writes. Each record is (little-endian):
//
//	u32 magic 'NWDL' | u64 ts_ms | u8 dir | u8 reserved[3] | u64 ssl_ptr | u32 len | payload[len]
//
// ssl_ptr is the game's SSL object, one per DTLS connection; a new login is a
// new connection. Records are written in time order.
package ledger

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	Magic      = 0x4C44574E // "NWDL"
	HeaderSize = 28

	offTime    = 4
	offSSL     = 16
	offLen     = 24
	maxPayload = 1 << 24 // larger than any DTLS record; a bigger length means damage
)

// Connection is one DTLS connection in a ledger.
type Connection struct {
	SSL     uint64 `json:"ssl"`
	FirstMs int64  `json:"first_ms"`
	LastMs  int64  `json:"last_ms"`
	Records int    `json:"records"`
	Bytes   int64  `json:"bytes"`
}

// Scan lists the connections in the order they first appear. A record cut
// short at the end (the game was killed mid-write) is ignored and counted in
// trailing.
func Scan(path string) (conns []Connection, trailing int64, err error) {
	index := map[uint64]int{}
	trailing, err = each(path, func(h header, raw []byte) error {
		i, ok := index[h.ssl]
		if !ok {
			i = len(conns)
			index[h.ssl] = i
			conns = append(conns, Connection{SSL: h.ssl, FirstMs: h.ts})
		}
		c := &conns[i]
		c.LastMs = h.ts
		c.Records++
		c.Bytes += int64(len(raw))
		return nil
	})
	return conns, trailing, err
}

// Sequential reports whether each connection ends before the next one
// starts. Only then can every part hold a whole connection without two parts
// covering the same time.
func Sequential(conns []Connection) bool {
	for i := 1; i < len(conns); i++ {
		if conns[i].FirstMs < conns[i-1].LastMs {
			return false
		}
	}
	return true
}

// Split writes each connection's records, unchanged and in order, to
// outPaths[i] for conns[i].
func Split(path string, conns []Connection, outPaths []string) error {
	if len(conns) != len(outPaths) {
		return errors.New("one output file per connection is needed")
	}
	files := make(map[uint64]*bufio.Writer, len(conns))
	var opened []*os.File
	defer func() {
		for _, f := range opened {
			f.Close()
		}
	}()
	for i, c := range conns {
		f, err := os.Create(outPaths[i])
		if err != nil {
			return err
		}
		opened = append(opened, f)
		files[c.SSL] = bufio.NewWriter(f)
	}
	if _, err := each(path, func(h header, raw []byte) error {
		w, ok := files[h.ssl]
		if !ok {
			return fmt.Errorf("record of unknown connection 0x%x", h.ssl)
		}
		_, err := w.Write(raw)
		return err
	}); err != nil {
		return err
	}
	for i, c := range conns {
		if err := files[c.SSL].Flush(); err != nil {
			return err
		}
		if err := opened[i].Close(); err != nil {
			return err
		}
	}
	opened = nil
	return nil
}

type header struct {
	ts  int64
	ssl uint64
}

// each calls fn with every whole record (header and payload together).
func each(path string, fn func(header, []byte) error) (trailing int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var off int64
	buf := make([]byte, HeaderSize)
	for {
		n, err := io.ReadFull(r, buf[:HeaderSize])
		if err == io.EOF {
			return 0, nil
		}
		if err == io.ErrUnexpectedEOF {
			return int64(n), nil
		}
		if err != nil {
			return 0, err
		}
		if binary.LittleEndian.Uint32(buf) != Magic {
			return 0, fmt.Errorf("ledger.bin is damaged at byte %d (no record start)", off)
		}
		size := binary.LittleEndian.Uint32(buf[offLen:])
		if size > maxPayload {
			return 0, fmt.Errorf("ledger.bin is damaged at byte %d (record length %d)", off, size)
		}
		total := HeaderSize + int(size)
		if cap(buf) < total {
			grown := make([]byte, total)
			copy(grown, buf[:HeaderSize])
			buf = grown
		}
		buf = buf[:total]
		if n, err := io.ReadFull(r, buf[HeaderSize:]); err != nil {
			if err == io.ErrUnexpectedEOF || err == io.EOF {
				return int64(HeaderSize + n), nil
			}
			return 0, err
		}
		h := header{ts: int64(binary.LittleEndian.Uint64(buf[offTime:])), ssl: binary.LittleEndian.Uint64(buf[offSSL:])}
		if err := fn(h, buf); err != nil {
			return 0, err
		}
		off += int64(total)
	}
}
