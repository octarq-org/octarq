package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"strings"

	"github.com/Jungley8/led/internal/models"
	"golang.org/x/crypto/ssh"
)

func (h *Handler) listSSHKeys(w http.ResponseWriter, r *http.Request) {
	var keys []models.SSHKey
	h.orgDB(r).Order("created_at DESC").Find(&keys)
	writeJSON(w, http.StatusOK, keys)
}

type sshKeyDTO struct {
	Name string `json:"name"`
	Type string `json:"type"` // "rsa", "ed25519", "imported"
	Key  string `json:"key"`  // raw private key
}

func (h *Handler) createSSHKey(w http.ResponseWriter, r *http.Request) {
	var d sshKeyDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	var privPEM []byte
	var pubKeyStr string

	if d.Type == "imported" {
		privPEM = []byte(d.Key)
		// Parse it to validate and get public key
		signer, err := ssh.ParsePrivateKey(privPEM)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid private key: "+err.Error())
			return
		}
		pubKeyStr = string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	} else if d.Type == "ed25519" {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to generate key")
			return
		}
		b, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to marshal key")
			return
		}
		privPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b})

		sshPub, _ := ssh.NewPublicKey(pub)
		pubKeyStr = string(ssh.MarshalAuthorizedKey(sshPub))
	} else if d.Type == "rsa" {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to generate key")
			return
		}
		b := x509.MarshalPKCS1PrivateKey(priv)
		privPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: b})

		sshPub, _ := ssh.NewPublicKey(&priv.PublicKey)
		pubKeyStr = string(ssh.MarshalAuthorizedKey(sshPub))
	} else {
		writeErr(w, http.StatusBadRequest, "invalid key type")
		return
	}

	encKey, err := h.cipher.Encrypt(privPEM)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to encrypt key")
		return
	}

	k := models.SSHKey{
		OrgID: h.orgID(r),
		Name:    d.Name,
		Type:    d.Type,
		Key:     string(encKey),
		PubKey:  strings.TrimSpace(pubKeyStr),
	}
	if err := h.db.Create(&k).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save key")
		return
	}

	// For a newly generated key, we can return the raw private key once so the user can save it if they want.
	if d.Type != "imported" {
		resp := struct {
			models.SSHKey
			RawPrivateKey string `json:"rawPrivateKey"`
		}{k, string(privPEM)}
		writeJSON(w, http.StatusCreated, resp)
		return
	}

	writeJSON(w, http.StatusCreated, k)
}

func (h *Handler) deleteSSHKey(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	// Verify ownership before checking usage.
	var key models.SSHKey
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&key).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var count int64
	h.db.Model(&models.VPS{}).Where("ssh_key_id = ?", id).Count(&count)
	if count > 0 {
		writeErr(w, http.StatusConflict, "key is in use by a VPS")
		return
	}
	h.db.Delete(&models.SSHKey{}, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
