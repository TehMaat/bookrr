package unit3d

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strconv"
)

var errBadTorrent = errors.New("invalid .torrent file")

// InfoHash returns the v1 info hash (SHA-1 of the bencoded "info"
// dictionary) of a .torrent file.
func InfoHash(data []byte) (string, error) {
	info, err := infoDict(data)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(info)
	return hex.EncodeToString(sum[:]), nil
}

// infoDict returns the bencoded "info" dictionary of a .torrent file.
func infoDict(data []byte) ([]byte, error) {
	if len(data) == 0 || data[0] != 'd' {
		return nil, errBadTorrent
	}
	i := 1
	for i < len(data) && data[i] != 'e' {
		key, next, err := bstring(data, i)
		if err != nil {
			return nil, err
		}
		end, err := skip(data, next, 0)
		if err != nil {
			return nil, err
		}
		if key == "info" {
			if data[next] != 'd' {
				return nil, errBadTorrent
			}
			return data[next:end], nil
		}
		i = end
	}
	return nil, errors.New("the .torrent file has no info dictionary")
}

// TorrentMeta is what bookrr reads of a .torrent file.
type TorrentMeta struct {
	// The v1 info hash, lowercase hex: the hash qBittorrent gives it.
	Hash string
	Name string
	// Total size of its files, in bytes.
	Size int64
}

// ReadTorrent reads info hash, name and size of a .torrent file.
func ReadTorrent(data []byte) (TorrentMeta, error) {
	info, err := infoDict(data)
	if err != nil {
		return TorrentMeta{}, err
	}
	sum := sha1.Sum(info)
	m := TorrentMeta{Hash: hex.EncodeToString(sum[:])}
	err = dictEach(info, 0, func(key string, at, end int) error {
		var err error
		switch key {
		case "name":
			if info[at] >= '0' && info[at] <= '9' {
				m.Name, _, err = bstring(info, at)
			}
		case "length":
			var n int64
			if n, err = bint(info, at); err == nil && n > 0 {
				m.Size += n
			}
		case "files":
			// A list of dictionaries, each with the length of a file.
			if info[at] != 'l' {
				return errBadTorrent
			}
			for j := at + 1; j < end-1; {
				next, err := skip(info, j, 1)
				if err != nil {
					return err
				}
				if info[j] == 'd' {
					if err := dictEach(info, j, func(k string, at, _ int) error {
						if k != "length" {
							return nil
						}
						n, err := bint(info, at)
						if err == nil && n > 0 {
							m.Size += n
						}
						return err
					}); err != nil {
						return err
					}
				}
				j = next
			}
		}
		return err
	})
	return m, err
}

// dictEach calls fn with every key of the dictionary starting at i, and
// where its value starts and ends.
func dictEach(b []byte, i int, fn func(key string, at, end int) error) error {
	if i >= len(b) || b[i] != 'd' {
		return errBadTorrent
	}
	j := i + 1
	for j < len(b) && b[j] != 'e' {
		key, next, err := bstring(b, j)
		if err != nil {
			return err
		}
		end, err := skip(b, next, 1)
		if err != nil {
			return err
		}
		if err := fn(key, next, end); err != nil {
			return err
		}
		j = end
	}
	if j >= len(b) {
		return errBadTorrent
	}
	return nil
}

// bint reads the integer starting at i ("i42e").
func bint(b []byte, i int) (int64, error) {
	if i >= len(b) || b[i] != 'i' {
		return 0, errBadTorrent
	}
	for j := i + 1; j < len(b); j++ {
		if b[j] == 'e' {
			n, err := strconv.ParseInt(string(b[i+1:j]), 10, 64)
			if err != nil {
				return 0, errBadTorrent
			}
			return n, nil
		}
	}
	return 0, errBadTorrent
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
