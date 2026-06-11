package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
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
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelay(),
		libp2p.EnableAutoRelayWithStaticRelays(bsPeers),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			var err error
			kadDHT, err = dht.New(ctx, h, dht.Mode(dht.ModeAuto))
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

func generateInviteCode(peerID string, password string) string {
	raw := peerID + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeInviteCode(code string) (peerID string, password string, err error) {
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

	fmt.Println("starting node...")
	h, kadDHT, err := startNodeWithDHT(ctx)
	if err != nil {
		fmt.Println("error starting node:", err)
		return
	}
	defer h.Close()

	// connect to bootstrap
	fmt.Println("connecting to network...")
	connectToBootstrap(ctx, h)

	// bootstrap DHT
	fmt.Println("bootstrapping DHT...")
	if err := kadDHT.Bootstrap(ctx); err != nil {
		fmt.Println("DHT bootstrap error:", err)
	}

	// wait for DHT and NAT to be ready
	fmt.Println("discovering public address...")
	time.Sleep(6 * time.Second)

	h.SetStreamHandler(protocol, func(s network.Stream) {
		welcome := fmt.Sprintf("system|%s|room created by %s\n", time.Now().Format("15:04"), username)
		_, _ = s.Write([]byte(welcome))
		go handlePeer(s, username, password)
	})

	// invite code now contains peer ID only — no IP
	code := generateInviteCode(h.ID().String(), password)

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

	peerIDStr, password, err := decodeInviteCode(code)
	if err != nil {
		fmt.Println("invalid invite code")
		return
	}

	targetID, err := peer.Decode(peerIDStr)
	if err != nil {
		fmt.Println("invalid peer ID:", err)
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
	if err := kadDHT.Bootstrap(ctx); err != nil {
		fmt.Println("DHT bootstrap error:", err)
	}

	// wait for DHT to be ready
	time.Sleep(8 * time.Second)

	h.SetStreamHandler(protocol, func(s network.Stream) {
		go handlePeer(s, username, password)
	})

	// look up peer addresses from DHT
	fmt.Println("looking up peer on DHT...")
	peerInfo, err := kadDHT.FindPeer(ctx, targetID)
	if err != nil {
		fmt.Println("peer not found on DHT:", err)
		fmt.Println("trying direct connection anyway...")
		peerInfo = peer.AddrInfo{ID: targetID}
	}

	// clear backoff and attempt connection with retries
	fmt.Println("connecting to room...")
	var connected bool
	for i := 0; i < 3; i++ {
		h.Network().(*swarm.Swarm).Backoff().Clear(targetID)
		if err := h.Connect(ctx, peerInfo); err == nil {
			connected = true
			break
		}
		fmt.Printf("retrying... (%d/3)\n", i+1)
		time.Sleep(2 * time.Second)
	}

	if !connected {
		fmt.Println("connection failed. make sure creator is online and try again.")
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