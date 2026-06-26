package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/Jungley8/led/internal/models"
	"golang.org/x/crypto/ssh"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for this tool, assuming dashboard runs on same host
	},
}

// terminalMessage is used for sending/receiving terminal resizing.
type terminalMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
	Data string `json:"data"`
}

func (h *Handler) vpsTerminal(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}

	var vps models.VPS
	if err := h.db.First(&vps, id).Error; err != nil {
		writeErr(w, http.StatusNotFound, "vps not found")
		return
	}

	var sshKey models.SSHKey
	if err := h.db.First(&sshKey, vps.SSHKeyID).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "ssh key not found")
		return
	}

	privKeyBytes, err := h.cipher.Decrypt(sshKey.Key)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to decrypt ssh key")
		return
	}

	signer, err := ssh.ParsePrivateKey(privKeyBytes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to parse ssh key")
		return
	}

	config := &ssh.ClientConfig{
		User: vps.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", vps.IP, vps.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "ssh dial failed: "+err.Error())
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "ssh session failed: "+err.Error())
		return
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "request pty failed: "+err.Error())
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return
	}

	if err := session.Shell(); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "shell failed: "+err.Error())
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Handle stdout
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				msg := terminalMessage{Type: "data", Data: string(buf[:n])}
				b, _ := json.Marshal(msg)
				conn.WriteMessage(websocket.TextMessage, b)
			}
			if err != nil {
				break
			}
		}
	}()

	// Handle stderr
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				msg := terminalMessage{Type: "data", Data: string(buf[:n])}
				b, _ := json.Marshal(msg)
				conn.WriteMessage(websocket.TextMessage, b)
			}
			if err != nil {
				break
			}
		}
	}()

	// Handle input from websocket
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg terminalMessage
		if err := json.Unmarshal(message, &msg); err == nil {
			if msg.Type == "data" {
				stdin.Write([]byte(msg.Data))
			} else if msg.Type == "resize" {
				session.WindowChange(msg.Rows, msg.Cols)
			}
		}
	}
}
