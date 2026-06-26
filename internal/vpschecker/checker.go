package vpschecker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/models"
	"github.com/Jungley8/led/internal/notify"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

type Checker struct {
	db     *gorm.DB
	cipher *crypto.Cipher
}

func New(db *gorm.DB, cipher *crypto.Cipher) *Checker {
	return &Checker{
		db:     db,
		cipher: cipher,
	}
}

func (c *Checker) Start(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	c.checkAll() // check once on startup

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkAll()
		}
	}
}

func (c *Checker) checkAll() {
	var list []models.VPS
	if err := c.db.Find(&list).Error; err != nil {
		slog.Error("vps checker: failed to list vps", "err", err)
		return
	}

	for _, vps := range list {
		c.checkOne(vps)
	}
}

func (c *Checker) checkOne(vps models.VPS) {
	now := time.Now()
	online := c.dial(vps)

	prevStatus := vps.Status

	if online {
		vps.FailCount = 0
		vps.Status = "online"
	} else {
		vps.FailCount++
		if vps.FailCount >= 3 {
			vps.Status = "offline"
		}
	}

	vps.LastChecked = &now
	c.db.Save(&vps)

	// Send notifications on state changes
	if prevStatus == "online" && vps.Status == "offline" {
		msg := fmt.Sprintf("🔴 VPS Offline Alert: %s (%s) has failed 3 consecutive health checks.", vps.Name, vps.IP)
		slog.Info("vps checker: offline alert", "vps", vps.Name)
		go c.broadcast(msg)
	} else if prevStatus == "offline" && vps.Status == "online" {
		msg := fmt.Sprintf("🟢 VPS Recovery: %s (%s) is back online.", vps.Name, vps.IP)
		slog.Info("vps checker: recovery alert", "vps", vps.Name)
		go c.broadcast(msg)
	}
}

func (c *Checker) broadcast(msg string) {
	var channels []models.NotificationChannel
	c.db.Where("enabled = ?", true).Find(&channels)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, ch := range channels {
		_ = notify.Send(ctx, ch.Type, ch.Config, msg)
	}
}

func (c *Checker) dial(vps models.VPS) bool {
	var sshKey models.SSHKey
	if err := c.db.First(&sshKey, vps.SSHKeyID).Error; err != nil {
		return false
	}

	privKeyBytes, err := c.cipher.Decrypt(sshKey.Key)
	if err != nil {
		return false
	}

	signer, err := ssh.ParsePrivateKey(privKeyBytes)
	if err != nil {
		return false
	}

	config := &ssh.ClientConfig{
		User: vps.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", vps.IP, vps.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return false
	}
	client.Close()
	return true
}
