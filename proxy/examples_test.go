package proxy_test

import (
	"github.com/cnelson/evaporation/proxy"
	"log"
)

func ExampleNewTorrentProxy() {
	p, err := proxy.NewTorrentProxy(&proxy.Config{
		TorrentURL: "magnet:?xt=urn:btih:adecafcafeadecafcafeadecafcafeadecafcafe",
	})
	defer p.Close()

	if err != nil {
		log.Fatal(err)
	}

	log.Print(p.URL())

	// Blocks forever
	p.Run()
}
