package proxy

import (
	"errors"
	"github.com/anacrolix/torrent"
	"io"
)

// Impelment the ReadSeeker interface for a given file in the torrent.
type torrentReadSeeker struct {
	Reader *torrent.Reader
	File   *torrent.File
}

// Read the requested data from a file in the torrent.
//
// This will block until the requested data has been downloaded from the swarm.
func (trs *torrentReadSeeker) Read(p []byte) (n int, err error) {
	// if there was no seek before the call to us
	// make sure we are at byte 0 of the file
	if trs.Reader.CurrentPos() < trs.File.Offset() {
		trs.Seek(0, io.SeekStart)
	}

	bufsize := int64(len(p))

	eof := trs.File.Offset() + trs.File.Length()
	if trs.Reader.CurrentPos()+bufsize > eof {
		bufsize = eof - trs.Reader.CurrentPos()
	}

	if bufsize == 0 {
		return 0, errors.New("EOF")
	}

	buf := make([]byte, bufsize)

	trs.File.PrioritizeRegion(trs.Reader.CurrentPos()-trs.File.Offset(), int64(bufsize))

	trs.Reader.Read(buf)
	return copy(p, buf), err
}

// Adjust seek requests to deal with the offset for multi-file torrents.
//
// Because we only have a reader for the entire torrent, we need to adjust seeks to hide
// the offset from the caller.
//
// net.HTTP expects a file to start and 0, and will Seek to (0, io.SeekEnd) to check length
func (trs *torrentReadSeeker) Seek(offset int64, whence int) (int64, error) {

	if whence == io.SeekStart {
		max := trs.File.Offset() + trs.File.Length()
		offset = trs.File.Offset() + offset

		if offset > max {
			offset = max
		}
	}

	if whence == io.SeekEnd {
		offset = (trs.File.Offset() + trs.File.Length()) - offset
		if offset < trs.File.Offset() {
			offset = trs.File.Offset()
		}
		whence = io.SeekStart
	}

	pos, err := trs.Reader.Seek(offset, whence)

	pos = pos - trs.File.Offset()

	return pos, err

}
