// Provides a HTTP/REST interface to the contents of a torrent file
//
// Use NewTorrentProxy to create an instance.
package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/anacrolix/dht"
	"github.com/anacrolix/torrent"
)

// Use NewTorrentProxy to create
type TorrentProxy struct {
	config    *Config
	client    *torrent.Client
	torrent   *torrent.Torrent
	httperror chan error
}

// Proxy configuration.
//
// TorrentURL must be specified. All other configuration is optional.
type Config struct {
	// A URL to a torrrent file.  Supported Schemes are:
	//
	//   - magnet: The TorrentSpec will contain information decoded from the URL only
	//
	//   - http/https: A GET request will be made to this URL.
	//     The response to the request must include he torrent file with a 200 OK status code.
	TorrentURL string

	// The list of nodes to seed DHT lookups.
	// If not specified, DHT will be disabled.
	DHTNodes []string

	// host:port for the HTTP server.
	// If not specified, defaults to a random port on localhost.
	HTTPListenAddr string

	// host:port for the torrent client
	// If not specified, defaults to a random port on all interfaces.
	TorrentListenAddr string

	// Path to a directory in which torrent data will be stored.
	// If not specified, defaults to current directory.
	DataDir string
}

// The state of a given file in a torrent
type TorrentFile struct {
	// The path to the file
	Path string `json:"path"`
	// The total size of the file
	Length int64 `json:"length"`
	// The percentage of pieces needs for this file that have been downloaded
	// 0.0. = not downloaded, 1.0 = fully downloaded
	Complete float32 `json:"complete"`
}

// The state of the torrent being proxied
type TorrentStatus struct {
	// "pending" if we are still loading the info hash.
	// "ready" if we have enough info to start downloading
	Status string `json:"status"`
	// The infohash in hexstring format
	Hash string `json:"id"`
	// The name of the torrent
	Name string `json:"name"`
	// The state of each file in the torrent
	Files []*TorrentFile `json:"files"`
}

// Configure and strt the torrent client
func (p *TorrentProxy) startTorrentClient() (err error) {
	// make sure our DHT nodes are legit before starting
	resolvedDHTNodes, err := resolveDHTNodes(p.config.DHTNodes)
	if err != nil {
		return fmt.Errorf("Error resolving DHT node: %s", err)
	}

	nodht := false
	log.Printf("Initial DHT Nodes: %s", resolvedDHTNodes)
	if len(resolvedDHTNodes) == 0 {
		log.Print("No DHT nodes supplied. Disabling DHT.")
		nodht = true
	}

	// make sure we have a torrent before starting
	spec, err := torrentSpecFromURL(p.config.TorrentURL)
	if err != nil {
		return fmt.Errorf("Invalid torrent URL: %s", err)
	}

	log.Printf("Resolved torrent URL to: %s (%s)", spec.InfoHash, spec.DisplayName)

	// start our client
	client, err := torrent.NewClient(&torrent.Config{
		DataDir:    p.config.DataDir,
		ListenAddr: p.config.TorrentListenAddr,

		NoDHT: nodht,
		DHTConfig: dht.ServerConfig{
			StartingNodes: func() ([]dht.Addr, error) {
				return resolvedDHTNodes, nil
			},
		},
	})
	if err != nil {
		return
	}

	p.client = client

	// add the torrent
	t, _, err := p.client.AddTorrentSpec(spec)
	p.torrent = t

	return
}

// Configure and start the web server
func (p *TorrentProxy) startHTTPServer() (err error) {
	// we do this instead of listenandserve so we can trap any errors listening
	listener, err := net.Listen("tcp", p.config.HTTPListenAddr)
	if err != nil {
		return
	}
	// and also figure out where we ended up if we use the default of ":0" and the OS picks a port
	// update our struct to where we actually landed
	p.config.HTTPListenAddr = listener.Addr().String()

	p.httperror = make(chan error)

	go func() {
		p.httperror <- http.Serve(listener, p)
	}()

	return
}

// Return the URL for the websever.
//
// This can be used to find the webserver if it's started on a random port.
func (p *TorrentProxy) URL() string {
	return "http://" + p.config.HTTPListenAddr
}

// Block until the webserver stops.
func (p *TorrentProxy) Run() (err error) {
	err = <-p.httperror
	return
}

// Return Status information about the loaded torrent
func (p *TorrentProxy) Status() (s *TorrentStatus) {
	status := "pending"
	if p.torrent.Info() != nil {
		status = "ready"
	}

	s = &TorrentStatus{
		Status: status,
		Name:   p.torrent.Name(),
		Hash:   p.torrent.InfoHash().HexString(),
		Files:  make([]*TorrentFile, 0),
	}

	var total float32
	var complete float32

	for _, file := range p.torrent.Files() {
		total = 0
		complete = 0

		for _, state := range file.State() {
			total++
			if state.PieceState.Complete {
				complete++
			}
		}

		s.Files = append(s.Files, &TorrentFile{
			Path:     file.Path(),
			Length:   file.Length(),
			Complete: complete / total,
		})
	}

	return
}

// Implement Handler interface for net/http.Serve().  The following URLs are supported:
//   / - Return TorrentStatus as JSON
//
//   /path/to/file/in/torrent - Return the contents of the file, or 404 if it does not exist.
func (p *TorrentProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// if it's the / request, then serve status
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p.Status())

		log.Printf("%d %s", 200, r.URL.Path)
		return
	}

	//else try to serve the file requested
	var thefile torrent.File
	for _, file := range p.torrent.Files() {
		if file.Path() == r.URL.Path[1:] {
			thefile = file
			break
		}
	}

	// if there's no path, then the file they asked for isn't in this torrent
	if len(thefile.Path()) == 0 {
		log.Printf("%d %s", 404, r.URL.Path)

		http.Error(w, "File Not Found", 404)
		return
	}

	// serve te file
	thefile.Download()
	log.Printf("%d %s", 200, r.URL.Path)
	http.ServeContent(w, r, thefile.Path(), time.Now(), &torrentReadSeeker{Reader: p.torrent.NewReader(), File: &thefile})
}

// Closes the torrent client and all files.
func (p *TorrentProxy) Close() {
	if p.client != nil {
		p.client.Close()
		p.client = nil
		p.torrent = nil
	}
}

// Create an instance of the proxy.
func NewTorrentProxy(config *Config) (proxy *TorrentProxy, err error) {
	//comments here?
	if len(config.HTTPListenAddr) == 0 {
		config.HTTPListenAddr = "localhost:0"
	}
	if len(config.TorrentListenAddr) == 0 {
		config.TorrentListenAddr = ":0"
	}

	proxy = &TorrentProxy{
		config: config,
	}

	err = proxy.startTorrentClient()
	if err != nil {
		return
	}

	err = proxy.startHTTPServer()
	if err != nil {
		return
	}

	return
}
