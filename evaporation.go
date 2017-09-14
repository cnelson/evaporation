package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"strings"

	"github.com/cnelson/evaporation/proxy"

	"github.com/anacrolix/dht"
)

type multiValue []string

func (m *multiValue) String() string {
	return strings.Join(*m, ", ")
}

func (m *multiValue) Set(value string) error {
	*m = append(*m, value)
	return nil
}
func usage() {
	fmt.Printf("Usage: %s [OPTIONS] url\n", os.Args[0])
	fmt.Println("   url - A magnet url or http url to a .torrent file.")

	fmt.Println("OPTIONS:")
	flag.PrintDefaults()
}

func main() {
	var dhtNodes multiValue

	flag.Usage = usage
	flag.Var(&dhtNodes, "dht", "host:port to seed DHT. Can be specified more than once.")

	var httpaddr = flag.String("http", "localhost:0", `host:port for the HTTP server to listen on. Use ":port" to listen on all interfaces. `)
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	if len(dhtNodes) == 0 {
		nodes, _ := dht.GlobalBootstrapAddrs()
		for _, node := range nodes {
			dhtNodes = append(dhtNodes, node.String())
		}
	}

	proxy, err := proxy.NewTorrentProxy(&proxy.Config{
		DHTNodes:       dhtNodes,
		TorrentURL:     flag.Arg(0),
		HTTPListenAddr: *httpaddr,
	})

	if err != nil {
		log.Fatalf("Unable to start proxy: %s", err)
	}

	log.Printf("Proxy up at: %s", proxy.URL())
	proxy.Run()

}
