package utils

import (
	"crypto/rand"
	"encoding/binary"
	"time"
)

func GenerateID() int64 {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return time.Now().UnixNano()
	}
	return int64(binary.BigEndian.Uint64(b))
}
