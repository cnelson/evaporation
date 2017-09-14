package proxy

import (
	"net"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/anacrolix/dht"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

var _ = Describe("Helpers", func() {

	Describe("Resolving Torrent URLs", func() {
		var (
			spec *torrent.TorrentSpec
			err  error
		)

		Context("Failed URL Parsing", func() {
			var (
				inputUrl string
			)
			AfterEach(func() {
				spec, err = torrentSpecFromURL(inputUrl)
				Expect(err).To(HaveOccurred())
			})

			It("fails when no URL is provided", func() {
				inputUrl = ""
			})

			It("fails when given an invalid url", func() {
				inputUrl = "http://192.0.2.%31/this/is/invalid"
			})

			It("fails when given a schemeless url", func() {
				inputUrl = "/this/has/no/scheme"
			})
			It("fails when given an unsupported scheme", func() {
				inputUrl = "unknown://protocol/here"
			})

		})

		Context("Magnet URL decoding", func() {
			It("fails when given an malformed magnet URL", func() {
				spec, err = torrentSpecFromURL("magnet:?xt=urn:btih:this-is-not-valid-hex")
				Expect(err).To(HaveOccurred())
			})

			It("decodes when given a valid magnet URL", func() {
				hex := "adecafcafeadecafcafeadecafcafeadecafcafe"
				name := "some-title"

				spec, err = torrentSpecFromURL("magnet:?dn=" + name + "&xt=urn:btih:" + hex)

				Expect(err).To(Succeed())
				Expect(spec.InfoHash.HexString()).To(Equal(hex))
				Expect(spec.DisplayName).To(Equal(name))
			})
		})

		Context("When talking to an HTTP server", func() {
			var (
				baseUrl string
			)

			BeforeEach(func() {
				http.DefaultServeMux = new(http.ServeMux)
				http.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "File Not Found", 404)
				})

				http.HandleFunc("/not-a-torrent", func(w http.ResponseWriter, r *http.Request) {
					http.ServeFile(w, r, "testdata/not-a-torrent.txt")
				})

				http.HandleFunc("/a-torrent", func(w http.ResponseWriter, r *http.Request) {
					http.ServeFile(w, r, "testdata/sample.torrent")
				})

				listener, _ := net.Listen("tcp", "localhost:0")
				baseUrl = "http://" + listener.Addr().String()
				go http.Serve(listener, nil)
			})

			It("fails when given an unreachable url", func() {
				spec, err = torrentSpecFromURL("http://localhost:99999/")
				Expect(err).To(HaveOccurred())
			})

			It("fails when given a URL that doesn't return 200", func() {
				spec, err = torrentSpecFromURL(baseUrl + "/fail")
				Expect(err).To(HaveOccurred())
			})

			It("fails when given an URL that isn't a torrent", func() {
				spec, err = torrentSpecFromURL(baseUrl + "/not-a-torrent")
				Expect(err).To(HaveOccurred())
			})

			It("decodes the torrent when given an good URL", func() {
				mi, _ := metainfo.LoadFromFile("testdata/sample.torrent")
				info, _ := mi.UnmarshalInfo()

				spec, err = torrentSpecFromURL(baseUrl + "/a-torrent")

				Expect(err).To(Succeed())
				Expect(spec.InfoHash.HexString()).To(Equal(mi.HashInfoBytes().HexString()))
				Expect(spec.DisplayName).To(Equal(info.Name))
			})
		})
	})

	Describe("Resolving DHT Nodes", func() {
		var (
			nodes         []string
			resolvedNodes []dht.Addr
			err           error
		)

		Context("When no nodes are provided", func() {
			our, ourerr := resolveDHTNodes(nodes)
			upstream, uperr := dht.GlobalBootstrapAddrs()

			It("should use the default upstream list", func() {
				Expect(our).To(Equal(upstream))
				Expect(ourerr).To(Succeed())
				Expect(uperr).To(Succeed())
			})
		})

		Context("When valid hostnames are provided", func() {
			nodes = []string{"example.com:1234"}

			// resolve example.com
			addrs, _ := net.LookupHost("example.com")

			// append our port number
			for i, addr := range addrs {
				addrs[i] = addr + ":1234"
			}

			resolvedNodes, err = resolveDHTNodes(nodes)

			It("returns them resolved", func() {
				Expect(err).To(Succeed())
				Expect(addrs).To(ContainElement(resolvedNodes[0].String()))
			})
		})

		Context("When valid IP addresses are provided", func() {
			AfterEach(func() {
				resolvedNodes, err = resolveDHTNodes(nodes)
				Expect(err).To(Succeed())
				Expect(nodes[0]).To(Equal(resolvedNodes[0].String()))
			})

			It("returns ipv4 adresses", func() {
				nodes = []string{"192.0.2.1:1234"}
			})

			It("returns ipv6 adresses", func() {
				nodes = []string{"[2001:db8::1]:1234"}
			})

		})

		Context("When invalid values are provided", func() {
			AfterEach(func() {
				resolvedNodes, err = resolveDHTNodes(nodes)
				Expect(err).To(HaveOccurred())
			})

			It("fails when unresolvable hostnames are providfed", func() {
				nodes = []string{"this_is_invalid:1234"}
			})

			It("fails when no port is provided", func() {
				nodes = []string{"192.0.2.1"}
			})

			It("fails when a bad port is provided", func() {
				nodes = []string{"192.0.2.1:99999"}
			})
		})
	})
})
