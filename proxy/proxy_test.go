package proxy

import (
	"encoding/json"

	"io/ioutil"

	"net"
	"net/http"

	"os"

	"strings"

	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Proxy", func() {
	var (
		err error
		p   *TorrentProxy
	)

	Context("An incorrectly configured proxy", func() {
		AfterEach(func() {
			if p != nil {
				p.Close()
			}
		})
		It("returns an error when given bad DHT nodes", func() {
			nodes := []string{"127.0.0.1:99999"}
			p, err = NewTorrentProxy(&Config{
				DHTNodes: nodes,
			})

			Expect(err).To(MatchError(ContainSubstring("DHT")))
		})

		It("returns an error when given a bad torrent url", func() {
			p, err = NewTorrentProxy(&Config{})

			Expect(err).To(MatchError(ContainSubstring("Invalid torrent")))
		})

		It("returns an error when given bad http listen address", func() {
			p, err = NewTorrentProxy(&Config{
				TorrentURL:     "magnet:?xt=urn:btih:adecafcafeadecafcafeadecafcafeadecafcafe",
				HTTPListenAddr: "localhost:99999",
			})

			Expect(err).To(MatchError(ContainSubstring("invalid port")))
		})

		It("returns an error when given bad torrent listen address", func() {
			p, err = NewTorrentProxy(&Config{
				TorrentURL:        "magnet:?xt=urn:btih:adecafcafeadecafcafeadecafcafeadecafcafe",
				TorrentListenAddr: "localhost:99999",
			})

			Expect(err).To(MatchError(ContainSubstring("invalid port")))
		})

	})

	Context("DHTnodes", func() {
		It("disabled DHT when no nodes are provided", func() {
			p, err = NewTorrentProxy(&Config{
				TorrentURL:        "magnet:?xt=urn:btih:adecafcafeadecafcafeadecafcafeadecafcafe",
				TorrentListenAddr: "localhost:0",
			})

			Expect(err).To(Succeed())
			Expect(p.client.DHT()).To(BeNil())
		})

		It("enables DHT when no nodes are provided", func() {
			nodes := []string{"127.0.0.1:65535"}
			p, err = NewTorrentProxy(&Config{
				DHTNodes:          nodes,
				TorrentListenAddr: "localhost:0",
				TorrentURL:        "magnet:?xt=urn:btih:adecafcafeadecafcafeadecafcafeadecafcafe",
			})

			Expect(err).To(Succeed())
			Expect(p.client.DHT()).To(Not(BeNil()))
		})

	})

	Context("A correctly configured proxy", func() {
		BeforeEach(func() {
			os.RemoveAll("testdata/.torrent.bolt.db")

			http.DefaultServeMux = new(http.ServeMux)

			http.HandleFunc("/a-torrent", func(w http.ResponseWriter, r *http.Request) {
				http.ServeFile(w, r, "testdata/sample.torrent")
			})

			listener, _ := net.Listen("tcp", "localhost:0")
			torrentURL := "http://" + listener.Addr().String() + "/a-torrent"
			go http.Serve(listener, nil)

			p, err = NewTorrentProxy(&Config{
				TorrentURL:        torrentURL,
				TorrentListenAddr: "localhost:0",
				DataDir:           "testdata",
			})

			Expect(err).To(Succeed())

			// wait for torrent to be hashed
			// the fixure should have two complete files in it
			tries := 0
			for {
				completed := 0

				s := p.Status()

				for _, f := range s.Files {
					if f.Complete == 1 {
						completed++
					}
				}

				if completed == 2 {
					break
				}

				tries++

				if tries > 10 {
					Fail("timed out waiting for hash")
					return
				}

				time.Sleep(time.Second * 1)
			}

		})

		AfterEach(func() {
			p.Close()
		})

		It("Returns torrent status", func() {
			js, _ := json.Marshal(p.Status())

			resp, _ := http.Get(p.URL())
			defer resp.Body.Close()
			body, _ := ioutil.ReadAll(resp.Body)

			Expect(strings.TrimSpace(string(body))).To(Equal(string(js)))
		})

		It("Returns torrent content", func() {
			s := p.Status()

			source, _ := ioutil.ReadFile("testdata/" + s.Files[0].Path)

			resp, _ := http.Get(p.URL() + "/" + s.Files[0].Path)
			defer resp.Body.Close()
			body, _ := ioutil.ReadAll(resp.Body)

			Expect(body).To(Equal(source))

		})

		It("Returns 404 for unknown files", func() {
			resp, _ := http.Get(p.URL() + "/this-file-does-not-exist.txt")
			Expect(resp.StatusCode).To(Equal(404))
		})

		It("Blocks on the Run method until the channel is closed", func() {
			close(p.httperror)
			err = p.Run()

			Expect(err).To(Succeed())
		})
	})
})
