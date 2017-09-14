package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/anacrolix/dht"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// Convert magnet urls or http/https urls to torrent files into a TorrentSpec
func torrentSpecFromURL(input string) (output *torrent.TorrentSpec, err error) {
	if len(input) == 0 {
		return output, fmt.Errorf("URL not specified")
	}

	u, err := url.Parse(input)
	if err != nil {
		return
	}

	if u.Scheme == "" {
		return output, fmt.Errorf("Unable to parse URL")
	}
	// if it's a magnet scheme, then try to convert to spec, if it's malformed, we'll fail
	if u.Scheme == "magnet" {
		output, err = torrent.TorrentSpecFromMagnetURI(input)
		if err != nil {
			err = fmt.Errorf("Malformed magnet url: %s", err)
		}
		return
	}

	// if it's an HTTP url, then attempt to fetch it and convert to magnet
	// but if it's not either of those, bail we don't know what to do
	if u.Scheme != "http" && u.Scheme != "https" {
		return output, fmt.Errorf("Unknown URL scheme: %s", u.Scheme)
	}

	resp, err := http.Get(input)
	if err != nil {
		return output, fmt.Errorf("Error fetching: %s", err)
	}
	defer resp.Body.Close()

	// TODO: be more permissive on code here?
	if resp.StatusCode != 200 {
		return output, fmt.Errorf("%s", resp.Status)
	}

	// this will fail fast and not read the whole body if it's not a torrent file
	mi, err := metainfo.Load(resp.Body)
	if err != nil {
		return output, fmt.Errorf("Not a valid torrent file: %s", err)
	}

	output = torrent.TorrentSpecFromMetaInfo(mi)

	return
}

// If given a list of DHT nodes, then resolve those, and return in a format appropriate for the client
// If not list is provided, use the defaults provided by the client
func resolveDHTNodes(nodes []string) (resolvedDHTNodes []dht.Addr, err error) {
	for _, hostport := range nodes {
		addr, err := net.ResolveUDPAddr("udp", hostport)
		if err != nil {
			return resolvedDHTNodes, err
		}
		resolvedDHTNodes = append(resolvedDHTNodes, dht.NewAddr(addr))
	}

	if len(resolvedDHTNodes) == 0 {
		resolvedDHTNodes, err = dht.GlobalBootstrapAddrs()
	}

	return
}
