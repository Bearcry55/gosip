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
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/multiformats/go-multiaddr"
)

const protocol = "/gosip/1.0.0"

var (
	peers   []network.Stream
	peersMu sync.Mutex
)

// public libp2p bootstrap nodes
var bootstrapAddrs = []string{
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
}

func getBootstrapPeers() []peer.AddrInfo {
	var peers []peer.AddrInfo
	for _, addr := range bootstrapAddrs {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		peers = append(peers, *pi)
	}
	return peers
}

func connectToBootstrap(ctx context.Context, h host.Host) {
	bootstrap := getBootstrapPeers()
	for _, p := range bootstrap {
		go func(pi peer.AddrInfo) {
			_ = h.Connect(ctx, pi)
		}(p)
	}
}

func startNode() (host.Host, error) {
	bsPeers := getBootstrapPeers()

	h, err := libp2p.New(
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelay(),
		libp2p.EnableAutoRelayWithStaticRelays(bsPeers),
	)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func getBestAddr(h host.Host) string {
	// prefer non-loopback, non-private addresses first (public IP)
	for _, addr := range h.Addrs() {
		full := addr.String() + "/p2p/" + h.ID().String()
		if !strings.Contains(full, "127.0.0.1") &&
			!strings.Contains(full, "192.168.") &&
			!strings.Contains(full, "10.0.") {
			return full
		}
	}
	// fallback to any non-loopback
	for _, addr := range h.Addrs() {
		full := addr.String() + "/p2p/" + h.ID().String()
		if !strings.Contains(full, "127.0.0.1") {
			return full
		}
	}
	return h.Addrs()[0].String() + "/p2p/" + h.ID().String()
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

func generateInviteCode(addrStr string, password string) string {
	raw := addrStr + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeInviteCode(code string) (addrStr string, password string, err error) {
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

	h, err := startNode()
	if err != nil {
		fmt.Println("error starting node:", err)
		return
	}
	defer h.Close()

	// connect to bootstrap nodes
	fmt.Println("connecting to network...")
	connectToBootstrap(ctx, h)

	// wait for node to discover public address
	fmt.Println("discovering public address...")
	time.Sleep(5 * time.Second)

	h.SetStreamHandler(protocol, func(s network.Stream) {
		welcome := fmt.Sprintf("system|%s|room created by %s\n", time.Now().Format("15:04"), username)
		_, _ = s.Write([]byte(welcome))
		go handlePeer(s, username, password)
	})

	bestAddr := getBestAddr(h)
	code := generateInviteCode(bestAddr, password)

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

	addrStr, password, err := decodeInviteCode(code)
	if err != nil {
		fmt.Println("invalid invite code")
		return
	}

	h, err := startNode()
	if err != nil {
		fmt.Println("error starting node:", err)
		return
	}
	defer h.Close()

	// connect to bootstrap nodes
	fmt.Println("connecting to network...")
	connectToBootstrap(ctx, h)
	time.Sleep(3 * time.Second)

	h.SetStreamHandler(protocol, func(s network.Stream) {
		go handlePeer(s, username, password)
	})

	maddr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		fmt.Println("invalid address:", err)
		return
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		fmt.Println("invalid peer info:", err)
		return
	}

	// retry connection with backoff
	fmt.Println("connecting to room...")
	var connected bool
	for i := 0; i < 3; i++ {
		// clear backoff so libp2p retries
		h.Network().(*swarm.Swarm).Backoff().Clear(peerInfo.ID)
		if err := h.Connect(ctx, *peerInfo); err == nil {
			connected = true
			break
		}
		fmt.Printf("retrying... (%d/3)\n", i+1)
		time.Sleep(2 * time.Second)
	}

	if !connected {
		fmt.Println("connection failed. check the invite code or ask creator to resend.")
		return
	}

	s, err := h.NewStream(ctx, peerInfo.ID, protocol)
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