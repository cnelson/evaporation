package proxy

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/anacrolix/torrent"
)

var _ = Describe("TorrentReadSeeker", func() {
	var (
		c   *torrent.Client
		t   *torrent.Torrent
		f   torrent.File
		trs *torrentReadSeeker
		err error
	)
	BeforeEach(func() {
		c, err = torrent.NewClient(&torrent.Config{
			DataDir: "testdata",
		})

		t, err = c.AddTorrentFromFile("testdata/sample.torrent")
	})

	AfterEach(func() {
		t = nil
		c.Close()
	})

	Context("With a file with a zero offset", func() {
		BeforeEach(func() {
			f = t.Files()[0]

			trs = &torrentReadSeeker{
				Reader: t.NewReader(),
				File:   &f,
			}

			Expect(f.Offset()).To(Equal(int64(0)))
		})

		It("will not read past the end of the file into the next one", func() {
			pos, err := trs.Seek(10, 2)

			Expect(err).To(Succeed())
			Expect(pos).To(Equal(f.Length() - 10))

			buf := make([]byte, 100)

			size, err := trs.Read(buf)

			Expect(err).To(Succeed())
			Expect(size).To(Equal(10))

			//subsequent calls return EOF
			size, err = trs.Read(buf)

			Expect(err).To(MatchError("EOF"))
			Expect(size).To(Equal(0))

		})

	})

	Context("With a file with an offset", func() {
		BeforeEach(func() {
			f = t.Files()[1]

			trs = &torrentReadSeeker{
				Reader: t.NewReader(),
				File:   &f,
			}

			Expect(f.Offset()).To(BeNumerically(">", 0))
		})

		It("seeks to the the start and returns 0", func() {
			pos, err := trs.Seek(0, 0)

			Expect(err).To(Succeed())
			Expect(pos).To(Equal(int64(0)))
		})

		It("seeks to the the end", func() {
			pos, err := trs.Seek(0, 2)

			Expect(err).To(Succeed())
			Expect(pos).To(Equal(f.Length()))
		})

		It("will not seek out of bounds", func() {
			pos, err := trs.Seek(f.Length()+100, 2)

			Expect(err).To(Succeed())
			Expect(pos).To(Equal(int64(0)))

			pos, err = trs.Seek(f.Length()+100, 0)

			Expect(err).To(Succeed())
			Expect(pos).To(Equal(f.Length()))
		})

		It("seeks relative", func() {
			pos, err := trs.Seek(10, 0)

			Expect(err).To(Succeed())
			Expect(pos).To(Equal(int64(10)))

			pos, err = trs.Seek(10, 1)

			Expect(err).To(Succeed())
			Expect(pos).To(Equal(int64(20)))
		})

		It("reads the correct content", func() {
			// read the original file from disk
			fh, err := os.Open("testdata/" + f.Path())
			defer fh.Close()

			startbuf := make([]byte, 100)
			midbuf := make([]byte, 100)

			fh.Read(startbuf)

			fh.Seek(200, 0)
			fh.Read(midbuf)

			// reading without seeking gets us the start of the file
			trsBuf := make([]byte, 100)
			size, err := trs.Read(trsBuf)

			Expect(err).To(Succeed())
			Expect(size).To(Equal(100))
			Expect(trsBuf).To(Equal(startbuf))

			// seeking to a location returns the right data
			pos, err := trs.Seek(200, 0)

			Expect(err).To(Succeed())
			Expect(pos).To(Equal(int64(200)))

			size, err = trs.Read(trsBuf)

			Expect(err).To(Succeed())
			Expect(size).To(Equal(100))

			Expect(trsBuf).To(Equal(midbuf))
		})
	})
})
