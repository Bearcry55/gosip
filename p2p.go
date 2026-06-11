package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/multiformats/go-multiaddr"
)

const protocol = "/gosip/1.0.0"

var (
	peers   []network.Stream
	peersMu sync.Mutex
)

var bootstrapAddrs = []string{
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
}

func getBootstrapPeers() []peer.AddrInfo {
	var result []peer.AddrInfo
	for _, addr := range bootstrapAddrs {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		result = append(result, *pi)
	}
	return result
}

func startNodeWithDHT(ctx context.Context) (host.Host, *dht.IpfsDHT, error) {
    bsPeers := getBootstrapPeers()
    var kadDHT *dht.IpfsDHT

    h, err := libp2p.New(
        // 1. Explicitly bind to standard ports to help with NAT mapping
        libp2p.ListenAddrStrings(
            "/ip4/0.0.0.0/tcp/4001",
            "/ip4/0.0.0.0/udp/4001/quic-v1",
        ),
        libp2p.NATPortMap(),
        libp2p.EnableNATService(),
        libp2p.EnableHolePunching(),
        
        // 2. CRITICAL: Force the app to scan for and use public relays 
        // because Termux cannot discover its own public routing data natively
        libp2p.EnableRelay(),
        libp2p.EnableAutoRelayWithStaticRelays(bsPeers),
        libp2p.ForceReachabilityPrivate(), // Forces node to look for relays immediately
        
        libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
            var err error
            // ModeClient bypasses the netlink permission error by not trying to act as a DHT server
            kadDHT, err = dht.New(ctx, h, dht.Mode(dht.ModeClient))
            return kadDHT, err
        }),
    )
    if err != nil {
        return nil, nil, err
    }
    return h, kadDHT, nil
}
func connectToBootstrap(ctx context.Context, h host.Host) {
	for _, p := range getBootstrapPeers() {
		go func(pi peer.AddrInfo) {
			_ = h.Connect(ctx, pi)
		}(p)
	}
}

// peerMeta is what we publish to ntfy.sh
type peerMeta struct {
	PeerID string   `json:"peer_id"`
	Addrs  []string `json:"addrs"`
}

func publishToNtfy(channel string, h host.Host) error {
    var addrs []string
    for _, addr := range h.Addrs() {
        addrStr := addr.String()
        
        // Skip local loopback completely as it's useless over the internet
        if strings.Contains(addrStr, "127.0.0.1") || strings.Contains(addrStr, "::1") {
            continue
        }
        
        full := addrStr + "/p2p/" + h.ID().String()
        addrs = append(addrs, full)
    }

    meta := peerMeta{
        PeerID: h.ID().String(),
        Addrs:  addrs,
    }

    data, err := json.Marshal(meta)
    if err != nil {
        return err
    }

    url := "https://ntfy.sh/" + channel
    resp, err := http.Post(url, "application/json", strings.NewReader(string(data)))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil
}
// fetchFromNtfy reads creator's peer info from ntfy.sh channel
func fetchFromNtfy(channel string) (*peerMeta, error) {
	url := "https://ntfy.sh/" + channel + "/raw?poll=1"
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// find the last valid JSON line
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var meta peerMeta
		if err := json.Unmarshal([]byte(line), &meta); err == nil {
			return &meta, nil
		}
	}
	return nil, fmt.Errorf("no valid peer info found")
}

func generateInviteCode(channel string, password string) string {
	raw := channel + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeInviteCode(code string) (channel string, password string, err error) {
	decoded, err := base64.StdEncoding.DecodeString(code)
	if err != nil {
		return "", "", err
	}
	idx := strings.LastIndex(string(decoded), ":")
	if idx == -1 {
		return "", "", fmt.Errorf("invalid code")
	}
	return string(decoded)[:idx], string(decoded)[idx+1:], nil
}

func addPeer(s network.Stream) {
	peersMu.Lock()
	peers = append(peers, s)
	peersMu.Unlock()
}

func removePeer(s network.Stream) {
	peersMu.Lock()
	defer peersMu.Unlock()
	for i, p := range peers {
		if p == s {
			peers = append(peers[:i], peers[i+1:]...)
			break
		}
	}
}

func broadcast(msg string, exclude network.Stream) {
	peersMu.Lock()
	defer peersMu.Unlock()
	for _, p := range peers {
		if exclude != nil && p.Conn().RemotePeer() == exclude.Conn().RemotePeer() {
			continue
		}
		_, _ = p.Write([]byte(msg))
	}
}

func handlePeer(s network.Stream, username string, password string) {
	addPeer(s)
	defer func() {
		removePeer(s)
		s.Close()
	}()

	scanner := bufio.NewScanner(s)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if isDuplicate(line) {
			continue
		}
		broadcast(line+"\n", s)
		sender, timestamp, msg := parsemessage(line)
		if sender == username {
			continue
		}
		if sender == "system" {
			fmt.Printf("\n\033[33m[system]\033[0m: %s\n> ", msg)
			continue
		}
		decrypted := DecryptMessage(msg, password)
		if decrypted == "" {
			continue
		}
		color := getcolor(sender)
		fmt.Printf("\n\033[90m[%s]\033[0m %s[%s]\033[0m: %s\n> ", timestamp, color, sender, decrypted)
	}
}

func p2pCreateRoom(username string) {
	ctx := context.Background()
	password := generatepassword()

	// random ntfy channel for discovery
	channel := "gosip-" + generatepassword()

	fmt.Println("starting node...")
	h, kadDHT, err := startNodeWithDHT(ctx)
	if err != nil {
		fmt.Println("error starting node:", err)
		return
	}
	defer h.Close()

	fmt.Println("connecting to network...")
	connectToBootstrap(ctx, h)

	fmt.Println("bootstrapping DHT...")
	_ = kadDHT.Bootstrap(ctx)

	// wait for addresses to be discovered
	fmt.Println("discovering addresses...")
	time.Sleep(30 * time.Second)

	// publish to ntfy.sh
	fmt.Println("publishing location...")
	if err := publishToNtfy(channel, h); err != nil {
		fmt.Println("warning: failed to publish:", err)
	} else {
		fmt.Println("location published!")
	}

	// keep republishing every 30s in background
	go func() {
		for {
			time.Sleep(30 * time.Second)
			_ = publishToNtfy(channel, h)
		}
	}()

	h.SetStreamHandler(protocol, func(s network.Stream) {
		welcome := fmt.Sprintf("system|%s|room created by %s\n", time.Now().Format("15:04"), username)
		_, _ = s.Write([]byte(welcome))
		go handlePeer(s, username, password)
	})

	code := generateInviteCode(channel, password)
	fmt.Println("\n── share this invite code with your friends ──")
	fmt.Println(code)
	fmt.Println("(copy the code above and share it privately)")
	fmt.Println("──────────────────────────────────────────────")
	fmt.Println("waiting for peers to connect...")
	fmt.Println("\033[90mType :/quit to leave\033[0m")

	p2pGroupSend(username, password)
}

func p2pJoinRoom(username string) {
	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("paste invite code: ")
	code, _ := reader.ReadString('\n')
	code = strings.TrimSpace(code)

	channel, password, err := decodeInviteCode(code)
	if err != nil {
		fmt.Println("invalid invite code")
		return
	}

	fmt.Println("starting node...")
	h, kadDHT, err := startNodeWithDHT(ctx)
	if err != nil {
		fmt.Println("error starting node:", err)
		return
	}
	defer h.Close()

	fmt.Println("connecting to network...")
	connectToBootstrap(ctx, h)

	fmt.Println("bootstrapping DHT...")
	_ = kadDHT.Bootstrap(ctx)

	time.Sleep(20 * time.Second)

	h.SetStreamHandler(protocol, func(s network.Stream) {
		go handlePeer(s, username, password)
	})

	// fetch creator addresses from ntfy.sh
	fmt.Println("fetching creator location from ntfy.sh...")
	var meta *peerMeta
	for i := 0; i < 5; i++ {
		meta, err = fetchFromNtfy(channel)
		if err == nil {
			break
		}
		fmt.Printf("retrying fetch... (%d/5)\n", i+1)
		time.Sleep(2 * time.Second)
	}
	if meta == nil {
		fmt.Println("failed to get creator location")
		return
	}

	fmt.Println("found creator! connecting...")

	// build peer info from fetched addresses
	targetID, err := peer.Decode(meta.PeerID)
	if err != nil {
		fmt.Println("invalid peer ID")
		return
	}

	var maddrs []multiaddr.Multiaddr
	for _, addrStr := range meta.Addrs {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}
		maddrs = append(maddrs, ma)
	}

	peerInfo := peer.AddrInfo{
		ID:    targetID,
		Addrs: maddrs,
	}

	// connect with retries
	var connected bool
	for i := 0; i < 3; i++ {
		h.Network().(*swarm.Swarm).Backoff().Clear(targetID)
		if err := h.Connect(ctx, peerInfo); err == nil {
			connected = true
			break
		}
		fmt.Printf("retrying connection... (%d/3)\n", i+1)
		time.Sleep(2 * time.Second)
	}

	if !connected {
		fmt.Println("connection failed. make sure creator is online.")
		return
	}

	s, err := h.NewStream(ctx, targetID, protocol)
	if err != nil {
		fmt.Println("stream error:", err)
		return
	}

	addPeer(s)
	fmt.Println("connected!")

	joinMsg := fmt.Sprintf("system|%s|%s has joined!\n", time.Now().Format("15:04"), username)
	broadcast(joinMsg, nil)

	go handlePeer(s, username, password)
	p2pGroupSend(username, password)
}

func p2pGroupSend(username string, password string) {
	rl, err := newReadline()
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	for {
		text, err := rl.Readline()
		if err != nil {
			break
		}
		text = strings.TrimSpace(text)
		if text == ":/quit" {
			leaveMsg := fmt.Sprintf("system|%s|%s has left the room\n", time.Now().Format("15:04"), username)
			broadcast(leaveMsg, nil)
			break
		}
		if text == "" {
			continue
		}
		timestamp := time.Now().Format("15:04")
		encrypted := EncryptMessage(text, password)
		msg := fmt.Sprintf("%s|%s|%s\n", username, timestamp, encrypted)
		broadcast(msg, nil)
	}
}

var (
	seenMessages   = make(map[string]time.Time)
	seenMessagesMu sync.Mutex
)

func isDuplicate(msg string) bool {
	seenMessagesMu.Lock()
	defer seenMessagesMu.Unlock()
	now := time.Now()
	for k, t := range seenMessages {
		if now.Sub(t) > 5*time.Second {
			delete(seenMessages, k)
		}
	}
	if _, found := seenMessages[msg]; found {
		return true
	}
	seenMessages[msg] = now
	return false
}