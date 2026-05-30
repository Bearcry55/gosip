package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	// the program first greet the user the first interface
	fmt.Println("\033[36m" + `
██████╗  ██████╗ ███████╗██╗██████╗ 
██╔════╝ ██╔═══██╗██╔════╝██║██╔══██╗
██║  ███╗██║   ██║███████╗██║██████╔╝
██║   ██║██║   ██║╚════██║██║██╔═══╝ 
╚██████╔╝╚██████╔╝███████║██║██║     
 ╚═════╝  ╚═════╝ ╚══════╝╚═╝╚═╝    
` + "\033[0m")
fmt.Println("\033[90m        say it. forget it.\033[0m")
fmt.Println()
	fmt.Println("....welcome to Gosip the programe is started ... ")
	fmt.Println("1. create  a CHAT ROOM  ")
	fmt.Println("2. enter a room with CHAT ROOM ID   ")
	fmt.Println("chose a option (1 or 2 ):  ")
	//now need to variavble to store the option value
	reader := bufio.NewReader(os.Stdin)
	choice, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("there some error in taking the input ")
		return
	}
	choice = strings.TrimSpace(choice)
	switch choice {
	case "1":
		fmt.Println("give me a username you like : ")
		username, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("there some error in taking your username ")
			return
		}
		username = strings.TrimSpace(username)

		fmt.Println("give me a roomid you like : ")
		roomid, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("there some error in taking your room id ")
			return
		}
		roomid = strings.TrimSpace(roomid)

		fmt.Println("creating a chatroom now with room id ... ")
		creatingchatroom(username, roomid)

	case "2":
		fmt.Println("give me a username you like : ")
		username, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("there some error in taking your username ")
			return
		}
		username = strings.TrimSpace(username)
		fmt.Println("give me a roomid you like to enter  : ")
		roomid, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("there some error in taking your room id ")
			return
		}
		roomid = strings.TrimSpace(roomid)
		fmt.Println("enter room password: ")
		password, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("error reading password")
			return
		}
		password = strings.TrimSpace(password)
		fmt.Println("entering in the chatroom with room id... ")
		fmt.Println("hi", username, "\n welcome to Gosip ")
		joinchatroom(roomid, username, password)
	default:
		fmt.Println("\n  invalid option :( you choose ", choice, " please choose correct option . ")
	}

}
