package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

// mov spec: https://developer.apple.com/standards/qtff-2001.pdf
// Page 31-33 contain information used in this file

const appleEpochAdjustment = 2082844800

const (
	movieResourceAtomType   = "moov"
	movieHeaderAtomType     = "mvhd"
	referenceMovieAtomType  = "rmra"
	compressedMovieAtomType = "cmov"
)

var separator = "/"

func init() {
	if runtime.GOOS == "windows" {
		separator = "\\"
	}
}

func main() {
	dirpath := os.Args[1]
	filepathList := ReadFileAbsolutePath(dirpath)
	fmt.Println(filepathList)

	for _, path := range filepathList {
		func(path string) {
			var oldname, newname string
			fd, err := os.Open(path)
			if err != nil {
				panic(err)
			}
			defer func() {
				fd.Close()
				err = os.Rename(oldname, newname)
				fmt.Println(err)
			}()

			info, _ := fd.Stat()

			created, err := getVideoCreationTimeMetadata(fd)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			filename := info.Name()
			arr := strings.Split(filename, "_")
			idx := arr[len(arr)-1]
			nextname := fmt.Sprintf("VID_%s_%s", created.Format("20060102"), idx)
			fmt.Printf("Movie created at %s (%s) %s\n", path, created, nextname)
			oldname = path
			newname = strings.ReplaceAll(oldname, filename, nextname)
		}(path)
	}
}

func ReadFileAbsolutePath(path string) []string {
	ans := []string{}
	fd, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer fd.Close()
	s, err := fd.Stat()
	if err != nil {
		panic(err)
	}

	if s.IsDir() {
		dirabsolute := path + separator
		entries, err := os.ReadDir(dirabsolute)
		if err != nil {
			panic(err)
		}

		for i := 0; i < len(entries); i++ {
			entry := entries[i]
			ans = append(ans, ReadFileAbsolutePath(path+separator+entry.Name())...)
		}
		return ans
	}

	lowerName := strings.ToLower(s.Name())
	if strings.HasSuffix(lowerName, ".mov") || strings.HasSuffix(lowerName, ".mp4") {
		ans = append(ans, path)
	}
	return ans
}

func getVideoCreationTimeMetadata(videoBuffer io.ReadSeeker) (time.Time, error) {
	buf := make([]byte, 8)

	// Traverse videoBuffer to find movieResourceAtom
	for {
		// bytes 1-4 is atom size, 5-8 is type
		// Read atom
		if _, err := videoBuffer.Read(buf); err != nil {
			return time.Time{}, err
		}

		if bytes.Equal(buf[4:8], []byte(movieResourceAtomType)) {
			break // found it!
		}

		atomSize := binary.BigEndian.Uint32(buf) // check size of atom
		videoBuffer.Seek(int64(atomSize)-8, 1)   // jump over data and set seeker at beginning of next atom
	}

	// read next atom
	if _, err := videoBuffer.Read(buf); err != nil {
		return time.Time{}, err
	}

	atomType := string(buf[4:8]) // skip size and read type
	switch atomType {
	case movieHeaderAtomType:
		// read next atom
		if _, err := videoBuffer.Read(buf); err != nil {
			return time.Time{}, err
		}

		// byte 1 is version, byte 2-4 is flags, 5-8 Creation time
		appleEpoch := int64(binary.BigEndian.Uint32(buf[4:])) // Read creation time

		return time.Unix(appleEpoch-appleEpochAdjustment, 0).Local(), nil
	case compressedMovieAtomType:
		return time.Time{}, errors.New("Compressed video")
	case referenceMovieAtomType:
		return time.Time{}, errors.New("Reference video")
	default:
		return time.Time{}, errors.New("Did not find movie header atom (mvhd)")
	}
}
