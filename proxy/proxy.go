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

// The publically exposed configuration for our TorrentProxy
type Config struct {
	// The list of nodes to seed DHT lookups
	DHTNodes []string

	// The torrent URL to download can be a magnet URL
	// or a http/https URL to a torrent file which will be downloaded
	TorrentURL string

	// Where should the HTTP Server listen
	HTTPListenAddr string

	// Where should the Torrent client listen
	TorrentListenAddr string

	// where to store downloaded data
	DataDir string
}

type TorrentFile struct {
	Path   string `json:"path"`
	Length int64  `json:"length"`
	// 0 = not downloaded, 1 = fully downloaded
	Complete float32 `json:"complete"`
}

type TorrentStatus struct {
	// pending if we are still loading the info hash, ready if we can attempt streaming
	Status string         `json:"status"`
	Hash   string         `json:"id"`
	Name   string         `json:"name"`
	Files  []*TorrentFile `json:"files"`
}

type TorrentProxy struct {
	config    *Config
	client    *torrent.Client
	torrent   *torrent.Torrent
	httperror chan error
}

// Start our torrent client by resolving DHT nodes and the torrent URL
func (p *TorrentProxy) startTorrentClient() (err error) {
	// make sure our DHT nodes are legit before starting
	resolvedDHTNodes, err := resolveDHTNodes(p.config.DHTNodes)
	if err != nil {
		return fmt.Errorf("Error resolving DHT node: %s", err)
	}

	log.Printf("Initial DHT Nodes: %s", resolvedDHTNodes)

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

// Configure and start the Web Server
func (p *TorrentProxy) startHTTPServer() (err error) {
	// all request to the ServeHTTP function
	http.Handle("/", p)

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
		p.httperror <- http.Serve(listener, nil)
	}()

	return
}

// return where to find our webserver
func (p *TorrentProxy) URL() string {
	return "http://" + p.config.HTTPListenAddr
}

// block until webserver exists
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

// handle HTTP requests
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

	// stream the file
	thefile.Download()
	log.Printf("%d %s", 200, r.URL.Path)
	http.ServeContent(w, r, thefile.Path(), time.Now(), &torrentReadSeeker{Reader: p.torrent.NewReader(), File: &thefile})
}

func (p *TorrentProxy) Close() {
	if p.client != nil {
		p.client.Close()
		p.client = nil
		p.torrent = nil
	}
}

// Start a new proxy
func NewTorrentProxy(config *Config) (proxy *TorrentProxy, err error) {
	if len(config.HTTPListenAddr) == 0 {
		config.HTTPListenAddr = "localhost:0"
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
