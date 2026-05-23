package main

import (
	"bufio"
	"fmt"
	"net/http"
	
	"strings"
	"time"
	"github.com/chzyer/readline"
)

func listeningtomsg(roomid string, username string, password string) {
    for {
        url      := "https://ntfy.sh/" + roomid +"/raw"
        response, err := http.Get(url)
        if err != nil {
            continue // reconnect immediately
        }

        scanner := bufio.NewScanner(response.Body)
        for scanner.Scan() {
            Text := scanner.Text()
            if Text == "" {
                continue
            }

            sender, timestamp, msg := parsemessage(Text)
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

        response.Body.Close() // close before reconnecting
    }
}
func sendmessages(roomid string, Text string, username string, password string) {
	url := "https://ntfy.sh/" + roomid
    timestamp := time.Now().Format("15:04")
	var message string
	if username == "system" {
        // Included the timestamp into the system message format
        message = fmt.Sprintf("system|%s|%s", timestamp, Text)
    } else {
        encrypted := EncryptMessage(Text, password)
        // Updated to the requested format: username|timestamp|encrypted_text
        message = fmt.Sprintf("%s|%s|%s", username, timestamp, encrypted)
    }

	messages := strings.NewReader(message)
	response, err := http.Post(url, "text/plain", messages)
	if err != nil {
		fmt.Println("there is some problem sending the message")
		return
	}
	defer response.Body.Close()
}
func joinchatroom(roomid string, username string, password string) {
    fmt.Println("checking if room exists...")

    // poll existing messages
    resp, err := http.Get("https://ntfy.sh/" + roomid + "/raw?poll=1")
    if err != nil {
        fmt.Println("error connecting")
        return
    }
    defer resp.Body.Close()

    // read all existing messages
    scanner := bufio.NewScanner(resp.Body)
    found := false
    for scanner.Scan() {
        line := scanner.Text()
        if strings.Contains(line, "created the room") {
            found = true
            break
        }
    }

    if !found {
        fmt.Println("\033[31mroom not found. check your room ID.\033[0m")
        return
    }

    fmt.Println("room found! joining...")
    url  := "https://ntfy.sh/" + roomid
    body := strings.NewReader("system|joined|" + username)
    r, err := http.Post(url, "text/plain", body)
    if err != nil {
        fmt.Println("error joining room")
        return
    }
    defer r.Body.Close()

    fmt.Println("you have successfully joined:", roomid)
    startchat(roomid, username, password)
}

func startchat(roomid string, username string, password string) {
	fmt.Println("\033[90mType :/quit to leave the room\033[0m")
	rl, err := readline.NewEx(&readline.Config{
		Prompt: "\033[36m>\033[0m ",
	})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	go listeningtomsg(roomid, username, password)

	for {
		text, err := rl.Readline()
		if err != nil {
			break
		}
		text = strings.TrimSpace(text)

		if text == ":/quit" {
			sendmessages(roomid, username+" has left the room", "system", password)
			fmt.Println("leaving room...")
			break
		}

		if text == "" {
			continue
		}

		sendmessages(roomid, text, username, password)
	}
}

func parsemessage(raw string) (string, string, string) {
    parts := strings.SplitN(raw, "|", 3)
    if len(parts) == 3 {
        return parts[0], parts[1], parts[2] // username, time, msg
    }
    return "system", "", raw
}

var colors = []string{
    "\033[32m", // green
    "\033[33m", // yellow
    "\033[34m", // blue
    "\033[35m", // magenta
    "\033[36m", // cyan
    "\033[91m", // bright red
    "\033[92m", // bright green
    "\033[93m", // bright yellow
    "\033[94m", // bright blue
    "\033[95m", // bright magenta
    "\033[96m", // bright cyan
    "\033[97m", // bright white
}

var userColors = map[string]string{}
var reset = "\033[0m"

func getcolor(username string) string {
    if _, exists := userColors[username]; !exists {
        total := 0
        for _, c := range username {
            total += int(c)
        }
        userColors[username] = colors[total%len(colors)]
    }
    return userColors[username]
}