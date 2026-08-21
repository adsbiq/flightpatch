package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ReceiverInventory is the local, non-secret contract consumed by the
// InternetHealthIQ tuner. USB serials stay on the machine.
type ReceiverInventory struct {
	Key         string `json:"key"`
	Serial      string `json:"serial"`
	Product     string `json:"product,omitempty"`
	Index       int    `json:"index"`
	Role        string `json:"role"`
	FrequencyHz int64  `json:"frequencyHz"`
	BeastPort   int    `json:"beastPort,omitempty"`
	SBSPort     int    `json:"sbsPort,omitempty"`
	VDL2Port    int    `json:"vdl2Port,omitempty"`
	ReceiverID  string `json:"receiverId,omitempty"`
}

func receiverIdentity(deviceID, key string) ([]byte, string) {
	h := sha256.Sum256([]byte(deviceID + "|" + key))
	raw := append([]byte(nil), h[:8]...)
	x := fmt.Sprintf("%x", raw)
	return raw, fmt.Sprintf("%s-%s-%s", x[:8], x[8:12], x[12:16])
}

func writeReceiverInventory(cfg *DeviceConfig, receivers []ReceiverInventory) error {
	sort.Slice(receivers, func(i, j int) bool {
		if receivers[i].Role == receivers[j].Role {
			return receivers[i].Key < receivers[j].Key
		}
		return receivers[i].Role < receivers[j].Role
	})
	doc := struct {
		SchemaVersion int                 `json:"schemaVersion"`
		UpdatedAt     string              `json:"updatedAt"`
		Receivers     []ReceiverInventory `json:"receivers"`
	}{1, time.Now().UTC().Format(time.RFC3339), receivers}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(cfg.path)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, "receivers.json.tmp")
	dst := filepath.Join(dir, "receivers.json")
	if err = os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
