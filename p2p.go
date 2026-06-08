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
	"github.com/multiformats/go-multiaddr"
)

const protocol = "/gosip/1.0.0"

// holds all connected peers
var (
	peers   []network.Stream
	peersMu sync.Mutex
)

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
		// Prevent echoing the message back to the original sender's peer identity
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

func startNode() (host.Host, error) {
	h, err := libp2p.New(
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		return nil, err
	}
	return h, nil
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

		// BUG FIX: Check if we have already processed this exact message string
		if isDuplicate(line) {
			continue
		}

		// Relays incoming mesh data out to other connected peers 
		broadcast(line+"\n", s)

		sender, timestamp, msg := parsemessage(line)

		// Safety filtering for duplicate display prevention
		if sender == username {
			continue
		}

		// UI Parsing & Delivery 
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
	password := generatepassword()

	h, err := startNode()
	if err != nil {
		fmt.Println("error starting node:", err)
		return
	}
	defer h.Close()

	// accept ALL incoming peers
	h.SetStreamHandler(protocol, func(s network.Stream) {
		go handlePeer(s, username, password)
	})

	var bestAddr string
	for _, addr := range h.Addrs() {
		full := addr.String() + "/p2p/" + h.ID().String()
		if !strings.Contains(full, "127.0.0.1") {
			bestAddr = full
			break
		}
	}
	if bestAddr == "" {
		bestAddr = h.Addrs()[0].String() + "/p2p/" + h.ID().String()
	}

	code := generateInviteCode(bestAddr, password)
	fmt.Println("\n── share this invite code with your friends ──")
	fmt.Println(code)
	fmt.Println("(copy the code above and share it privately)")
	fmt.Println("──────────────────────────────────────────────")
	fmt.Println("waiting for peers to connect...")
	fmt.Println("\033[90mType :/quit to leave\033[0m")

	// creator sends messages too
	p2pGroupSend(username, password)
}

func p2pJoinRoom(username string) {
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

	// Allows clients that joined to listen to incoming broadcast streams from other peers
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

	fmt.Println("connecting...")
	if err := h.Connect(context.Background(), *peerInfo); err != nil {
		fmt.Println("connection failed:", err)
		return
	}

	s, err := h.NewStream(context.Background(), peerInfo.ID, protocol)
	if err != nil {
		fmt.Println("stream error:", err)
		return
	}

	addPeer(s)
	fmt.Println("connected!")

	// send join notification
	joinMsg := fmt.Sprintf("system|%s|%s has joined!\n", time.Now().Format("15:04"), username)
	_, _ = s.Write([]byte(joinMsg))

	// listen for incoming messages on outbound channel
	go handlePeer(s, username, password)

	// send messages
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

// isDuplicate returns true if the message was seen within the last 5 seconds
func isDuplicate(msg string) bool {
	seenMessagesMu.Lock()
	defer seenMessagesMu.Unlock()

	// Clean up old entries to prevent memory leaks
	now := time.Now()
	for k, t := range seenMessages {
		if now.Sub(t) > 5*time.Second {
			delete(seenMessages, k)
		}
	}

	// Check if this exact string was handled recently
	if _, found := seenMessages[msg]; found {
		return true
	}

	// Mark as seen
	seenMessages[msg] = now
	return false
}