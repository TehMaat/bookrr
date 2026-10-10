package unit3d

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strconv"
)

var errBadTorrent = errors.New("file .torrent non valido")

// InfoHash returns the v1 info hash (SHA-1 of the bencoded "info"
// dictionary) of a .torrent file.
func InfoHash(data []byte) (string, error) {
	if len(data) == 0 || data[0] != 'd' {
		return "", errBadTorrent
	}
	i := 1
	for i < len(data) && data[i] != 'e' {
		key, next, err := bstring(data, i)
		if err != nil {
			return "", err
		}
		end, err := skip(data, next, 0)
		if err != nil {
			return "", err
		}
		if key == "info" {
			if data[next] != 'd' {
				return "", errBadTorrent
			}
			sum := sha1.Sum(data[next:end])
			return hex.EncodeToString(sum[:]), nil
		}
		i = end
	}
	return "", errors.New("file .torrent senza dizionario info")
}

// bstring reads the byte string starting at i and returns it with the
// index right after it.
func bstring(b []byte, i int) (string, int, error) {
	colon := i
	for colon < len(b) && b[colon] >= '0' && b[colon] <= '9' {
		colon++
	}
	if colon == i || colon >= len(b) || b[colon] != ':' {
		return "", 0, errBadTorrent
	}
	n, err := strconv.Atoi(string(b[i:colon]))
	if err != nil || n > len(b)-colon-1 {
		return "", 0, errBadTorrent
	}
	start := colon + 1
	return string(b[start : start+n]), start + n, nil
}

// skip returns the index right after the bencoded value starting at i.
func skip(b []byte, i, depth int) (int, error) {
	if i >= len(b) || depth > 64 {
		return 0, errBadTorrent
	}
	switch c := b[i]; {
	case c == 'i':
		for j := i + 1; j < len(b); j++ {
			if b[j] == 'e' {
				return j + 1, nil
			}
		}
		return 0, errBadTorrent
	case c == 'l' || c == 'd':
		j := i + 1
		for j < len(b) && b[j] != 'e' {
			next, err := skip(b, j, depth+1)
			if err != nil {
				return 0, err
			}
			j = next
		}
		if j >= len(b) {
			return 0, errBadTorrent
		}
		return j + 1, nil
	case c >= '0' && c <= '9':
		_, next, err := bstring(b, i)
		return next, err
	default:
		return 0, errBadTorrent
	}
}
