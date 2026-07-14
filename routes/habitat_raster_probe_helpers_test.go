package routes

import (
	"bytes"
	"strconv"
)

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func probeItoa(i int) string { return strconv.Itoa(i) }
