package storage

import (
	"fmt"
	"io"
	"os"
)

// ValidateMP4Container checks the merged file begins with a valid MP4/MOV ftyp box.
func ValidateMP4Container(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	hdr := make([]byte, 12)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return fmt.Errorf("video file too small or unreadable")
	}
	// Bytes 4–7 must be "ftyp" (ISO BMFF / QuickTime).
	if string(hdr[4:8]) != "ftyp" {
		return fmt.Errorf("invalid video container (missing ftyp box)")
	}
	return nil
}
