package main
import (
   "net/http"
    "fmt"
	"strings"
	"bufio"
    "math/rand"
)
func creatingchatroom(username string,roomid string){
	fmt.Println("we are making a room with room id :",roomid,"and with username:",username)
       password := generatepassword()
    fmt.Println("Your room password:", password)
    fmt.Println("Share this password with your friend privately!")

	//lets create a post request now 
	url:="https://ntfy.sh/" +roomid
	body := strings.NewReader("system|" + username + " created the room")

	resp,err:=http.Post(url,"text/plain",body)
	if err!=nil {
		fmt.Println("there is error making the room with the id ")
		return
	}
    defer resp.Body.Close()
	fmt.Println("room created ")
	fmt.Println("waiting for some one to join")
	joinedUser := waitForJoin(roomid)
    fmt.Println(joinedUser + " has joined!")
    
    
startchat(roomid,username,password)
}
func waitForJoin(roomid string) string {
    url  := "https://ntfy.sh/" + roomid + "/raw"
    resp, err := http.Get(url)
    if err != nil {
        fmt.Println("error while waiting to some one join", err)
        return ""
    }
    defer resp.Body.Close()

    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        line := scanner.Text()
        parts := strings.SplitN(line, "|", 3)

        // check if it's a join message
        if len(parts) == 3 && parts[0] == "system" && parts[1] == "joined" {
            return parts[2] // returns the username who joined
        }
    }
    return ""
}

func generatepassword() string {
    return fmt.Sprintf("%06d", rand.Intn(1000000))
}