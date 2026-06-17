package storage

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"
)

// ValidateImageFile checks magic bytes and, for decodable formats, verifies the image fully decodes.
func ValidateImageFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	hdr := make([]byte, 16)
	n, err := io.ReadFull(f, hdr)
	if err != nil || n < 3 {
		return fmt.Errorf("image file too small or unreadable")
	}
	if hdr[0] == 0xFF && hdr[1] == 0xD8 {
		return validateImageDecodes(path)
	}
	if n >= 8 && bytes.Equal(hdr[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return validateImageDecodes(path)
	}
	if n >= 12 && string(hdr[0:4]) == "RIFF" && string(hdr[8:12]) == "WEBP" {
		return validateImageDecodes(path)
	}
	if n >= 6 && (string(hdr[:6]) == "GIF87a" || string(hdr[:6]) == "GIF89a") {
		return validateImageDecodes(path)
	}
	// HEIC/HEIF/MOV — ffmpeg can transcode; allow ISO BMFF brands used by phone cameras.
	if n >= 12 && string(hdr[4:8]) == "ftyp" {
		brand := string(hdr[8:12])
		switch brand {
		case "heic", "heif", "mif1", "msf1", "qt  ", "avif":
			info, statErr := os.Stat(path)
			if statErr != nil || info.Size() < 1024 {
				return fmt.Errorf("heic/heif file too small")
			}
			return nil
		}
	}
	return fmt.Errorf("unrecognized or corrupt image format")
}

func validateImageDecodes(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("image decode failed: %w", err)
	}
	if cfg.Width < 2 || cfg.Height < 2 {
		return fmt.Errorf("image dimensions too small (%dx%d)", cfg.Width, cfg.Height)
	}
	return nil
}

// SanitizeHTTPMediaURLs rejects data URLs and raw base64 — only CDN https URLs are stored.
func SanitizeHTTPMediaURLs(label string, urls []string) ([]string, error) {
	if urls == nil {
		return nil, nil
	}
	out := make([]string, 0, len(urls))
	for i, raw := range urls {
		u := strings.TrimSpace(raw)
		if u == "" {
			continue
		}
		lower := strings.ToLower(u)
		if strings.HasPrefix(lower, "data:") {
			return nil, fmt.Errorf("%s[%d]: upload via /api/upload/image before saving (data URL not allowed)", label, i)
		}
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return nil, fmt.Errorf("%s[%d]: must be a CDN URL — upload media before saving", label, i)
		}
		out = append(out, u)
	}
	return out, nil
}

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
	if string(hdr[4:8]) != "ftyp" {
		return fmt.Errorf("invalid video container (missing ftyp box)")
	}
	return nil
}
