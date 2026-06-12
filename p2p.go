package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

const protocol = "/gosip/1.0.0"

var (
	myDataChannel      *webrtc.DataChannel   // my outgoing channel
	peerDataChannels   []*webrtc.DataChannel // all peer channels
	dataChannelMu      sync.Mutex
	seenMessages       = make(map[string]time.Time)
	seenMessagesMu     sync.Mutex
)

type SignalingPayload struct {
	PeerID string `json:"peer_id"`
	Type   string `json:"type"`
	SDP    string `json:"sdp"`
}

func publishToNtfy(channel string, sdpType string, sdpText string, username string) error {
	payload := SignalingPayload{
		PeerID: username,
		Type:   sdpType,
		SDP:    sdpText,
	}
	data, err := json.Marshal(payload)
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

func fetchFromNtfy(channel string, expectedType string) (*SignalingPayload, error) {
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

	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var payload SignalingPayload
		if err := json.Unmarshal([]byte(line), &payload); err == nil {
			if payload.Type == expectedType {
				return &payload, nil
			}
		}
	}
	return nil, fmt.Errorf("no matching %s signal found", expectedType)
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

// sendToAll sends a message to all peer channels
func sendToAll(msg string) {
	dataChannelMu.Lock()
	defer dataChannelMu.Unlock()
	for _, dc := range peerDataChannels {
		_ = dc.SendText(msg)
	}
}

// setupIncomingChannel handles messages received FROM a peer
func setupIncomingChannel(dc *webrtc.DataChannel, username string, password string) {
	dataChannelMu.Lock()
	peerDataChannels = append(peerDataChannels, dc)
	dataChannelMu.Unlock()

	dc.OnClose(func() {
		dataChannelMu.Lock()
		for i, d := range peerDataChannels {
			if d == dc {
				peerDataChannels = append(peerDataChannels[:i], peerDataChannels[i+1:]...)
				break
			}
		}
		dataChannelMu.Unlock()
		fmt.Printf("\n\033[33m[system]\033[0m: a peer has disconnected\n> ")
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		line := strings.TrimSpace(string(msg.Data))
		if line == "" || isDuplicate(line) {
			return
		}

		sender, timestamp, textPayload := parsemessage(line)
		if sender == username {
			return
		}
		if sender == "system" {
			fmt.Printf("\n\033[33m[system]\033[0m: %s\n> ", textPayload)
			return
		}
		decrypted := DecryptMessage(textPayload, password)
		if decrypted == "" {
			return
		}
		color := getcolor(sender)
		fmt.Printf("\n\033[90m[%s]\033[0m %s[%s]\033[0m: %s\n> ", timestamp, color, sender, decrypted)
	})
}

func p2pCreateRoom(username string) {
	password := generatepassword()
	channel := "gosip-" + generatepassword()

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
			}},
		},
	}

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer pc.Close()

	// creator's outgoing channel
	dc, err := pc.CreateDataChannel(protocol, nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// this is MY outgoing channel
	dc.OnOpen(func() {
		dataChannelMu.Lock()
		myDataChannel = dc
		dataChannelMu.Unlock()
	})

	// listen for joiner's incoming channel
	pc.OnDataChannel(func(incoming *webrtc.DataChannel) {
		setupIncomingChannel(incoming, username, password)
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	if err = pc.SetLocalDescription(offer); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("discovering addresses...")
	<-webrtc.GatheringCompletePromise(pc)

	fmt.Println("publishing location...")
	if err := publishToNtfy(channel, "offer", pc.LocalDescription().SDP, username); err != nil {
		fmt.Println("failed to publish:", err)
		return
	}
	fmt.Println("location published!")

	code := generateInviteCode(channel, password)
	fmt.Println("\n── share this invite code with your friends ──")
	fmt.Println(code)
	fmt.Println("(copy the code above and share it privately)")
	fmt.Println("──────────────────────────────────────────────")
	fmt.Println("waiting for peers to connect...")
	fmt.Println("\033[90mType :/quit to leave\033[0m")

	// poll for answer
	var answerPayload *SignalingPayload
	for i := 0; i < 30; i++ {
		answerPayload, err = fetchFromNtfy(channel, "answer")
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if answerPayload == nil {
		fmt.Println("no one joined.")
		return
	}

	if err = pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerPayload.SDP,
	}); err != nil {
		fmt.Println("handshake error:", err)
		return
	}

	// wait for my channel to open
	for i := 0; i < 15; i++ {
		dataChannelMu.Lock()
		ready := myDataChannel != nil
		dataChannelMu.Unlock()
		if ready {
			break
		}
		time.Sleep(1 * time.Second)
	}

	dataChannelMu.Lock()
	ready := myDataChannel != nil
	dataChannelMu.Unlock()

	if !ready {
		fmt.Println("connection failed.")
		return
	}

	fmt.Println("connected!")
	p2pGroupSend(username, password)
}

func p2pJoinRoom(username string) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("paste invite code: ")
	code, _ := reader.ReadString('\n')
	code = strings.TrimSpace(code)

	channel, password, err := decodeInviteCode(code)
	if err != nil {
		fmt.Println("invalid invite code")
		return
	}

	fmt.Println("fetching creator location...")
	var offerPayload *SignalingPayload
	for i := 0; i < 5; i++ {
		offerPayload, err = fetchFromNtfy(channel, "offer")
		if err == nil {
			break
		}
		fmt.Printf("retrying... (%d/5)\n", i+1)
		time.Sleep(2 * time.Second)
	}
	if offerPayload == nil {
		fmt.Println("failed to get creator location")
		return
	}

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
			}},
		},
	}

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer pc.Close()

	// joiner's outgoing channel
	dc, err := pc.CreateDataChannel(protocol+"/reply", nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// this is MY outgoing channel
	dc.OnOpen(func() {
		dataChannelMu.Lock()
		myDataChannel = dc
		dataChannelMu.Unlock()
		// send join notification
		joinMsg := fmt.Sprintf("system|%s|%s has joined!\n", time.Now().Format("15:04"), username)
		_ = dc.SendText(joinMsg)
	})

	// listen for creator's incoming channel
	pc.OnDataChannel(func(incoming *webrtc.DataChannel) {
		setupIncomingChannel(incoming, username, password)
	})

	if err = pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerPayload.SDP,
	}); err != nil {
		fmt.Println("invalid offer:", err)
		return
	}

	fmt.Println("found creator! connecting...")

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	if err = pc.SetLocalDescription(answer); err != nil {
		fmt.Println("error:", err)
		return
	}

	<-webrtc.GatheringCompletePromise(pc)

	if err = publishToNtfy(channel, "answer", pc.LocalDescription().SDP, username); err != nil {
		fmt.Println("failed to send answer:", err)
		return
	}

	// wait for my channel to open
	for i := 0; i < 15; i++ {
		dataChannelMu.Lock()
		ready := myDataChannel != nil
		dataChannelMu.Unlock()
		if ready {
			break
		}
		time.Sleep(1 * time.Second)
	}

	dataChannelMu.Lock()
	ready := myDataChannel != nil
	dataChannelMu.Unlock()

	if !ready {
		fmt.Println("connection failed.")
		return
	}

	fmt.Println("connected!")
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
			dataChannelMu.Lock()
			if myDataChannel != nil {
				_ = myDataChannel.SendText(leaveMsg)
			}
			dataChannelMu.Unlock()
			break
		}
		if text == "" {
			continue
		}
		timestamp := time.Now().Format("15:04")
		encrypted := EncryptMessage(text, password)
		msg := fmt.Sprintf("%s|%s|%s\n", username, timestamp, encrypted)

		dataChannelMu.Lock()
		if myDataChannel != nil {
			_ = myDataChannel.SendText(msg)
		}
		dataChannelMu.Unlock()
	}
}

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