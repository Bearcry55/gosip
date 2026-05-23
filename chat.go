package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"
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

            sender, msg := parsemessage(Text)

            if sender == username {
                continue
            }

            if sender == "system" {
                fmt.Printf("\n[system]: %s\n> ", msg)
                continue
            }

            decrypted := DecryptMessage(msg, password)
            if decrypted == "" {
                continue
            }
            fmt.Printf("\n[%s]: %s\n> ", sender, decrypted)
        }

        response.Body.Close() // close before reconnecting
    }
}
func sendmessages(roomid string, Text string, username string, password string) {
	url := "https://ntfy.sh/" + roomid

	var message string
	if username == "system" {
		message = fmt.Sprintf("system|%s", Text)
	} else {
		encrypted := EncryptMessage(Text, password)
		message = fmt.Sprintf("%s|%s", username, encrypted)
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
	fmt.Println("trying to join the room wait a sec.....")
	url  := "https://ntfy.sh/" + roomid
	body := strings.NewReader("system|joined|" + username)
	resp, err := http.Post(url, "text/plain", body)
	if err != nil {
		fmt.Println("there is error joining the room")
		return
	}
	defer resp.Body.Close()
	fmt.Println("you have successfully joined in this chat room:", roomid)
	startchat(roomid, username, password)
}

func startchat(roomid string, username string, password string) {
	go listeningtomsg(roomid, username, password)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		scanner.Scan()
		text := scanner.Text()

		if text == "exit" {
			sendmessages(roomid, "has left the room", "system", password)
			fmt.Println("leaving room...")
			break
		}

		if text == "" {
			continue
		}

		sendmessages(roomid, text, username, password)
	}
}

func parsemessage(raw string) (string, string) {
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "system", raw
}