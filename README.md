# 🤫 Gosip

> Anonymous encrypted terminal chat. No accounts. No logs. No trace.

Gosip is a lightweight terminal-based chat app built in Go. Create a room, share the ID and password with a friend, and start chatting. Messages are encrypted end-to-end using AES-256. When you leave, it's gone.

---

## Features

- Anonymous — no accounts, no sign up
- End-to-end encrypted with AES-256 GCM
- Auto-generated room passwords
- Colored usernames in terminal
- Timestamps on every message
- Works on Linux, Mac and Windows
- Single binary — no dependencies for the user

---

## Demo

```
....welcome to gosip the program is started ...
1. create a CHAT ROOM
2. enter a room with CHAT ROOM ID
chose a option (1 or 2 ): 1

give me a username you like : bear
give me a roomid you like  : jungle

Your room password: 344312
Share this password with your friend privately!

room created
waiting for someone to join...
lion has joined!

> hey lion how are you man
[16:32] [lion]: i am good bro
> great to hear!
```

---

## Installation

### Linux & Mac

**Option 1 — Build from source:**
```bash
git clone https://github.com/Bearcry55/gosip
cd gosip
go build -o gosip
./gosip
```

**Option 2 — Download binary:(most recommeneded)**

Linux:
```bash
wget https://github.com/Bearcry55/gosip/releases/latest/download/gosip-linux
chmod +x gosip-linux
./gosip-linux
```

Mac:
```bash
wget https://github.com/Bearcry55/gosip/releases/latest/download/gosip-mac
chmod +x gosip-mac
./gosip-mac
```

Windows — download `gosip.exe` from:
**Option 3 — go install:**
```bash
go install github.com/Bearcry55/gosip@latest
```
Then run:
```bash
gosip
```

### Windows

```powershell
git clone https://github.com/Bearcry55/gosip
cd gosip
go build -o gosip.exe
./gosip.exe
```

---

## Requirements

- Go 1.21 or higher → https://golang.org/dl

---

## How It Works

```
Create Room  →  auto generates room ID + password
              →  POST system message to ntfy.sh
              →  wait for someone to join

Join Room    →  enter room ID + password
              →  POST join message to ntfy.sh
              →  start chatting

Messages     →  encrypted with AES-256 GCM before sending
              →  stored temporarily on ntfy.sh as gibberish
              →  decrypted only by users with correct password
```

Without the password anyone can see the room but only reads gibberish.

---

## Commands

| Command | Action |
|---|---|
| `:/quit` | Leave the room |

---

## Privacy

- Messages are encrypted before leaving your machine
- Room password never sent over the network
- No user accounts or registration
- Messages expire automatically on ntfy.sh after 12 hours
- No message history stored locally

---

## Built With

- `Go` — core language
- `ntfy.sh` — temporary message transport
- `crypto/aes` — AES-256 GCM encryption
- `github.com/chzyer/readline` — terminal input handling

---

## Roadmap

- [ ] TUI interface
- [ ] Room expiration control  
- [ ] File sharing
- [ ] LAN mode (no internet needed)
- [ ] Tor mode

---

## Author

Built by [@Bearcry55](https://github.com/Bearcry55) as a Go learning project.

---

> Gosip. Say it. Forget it.
