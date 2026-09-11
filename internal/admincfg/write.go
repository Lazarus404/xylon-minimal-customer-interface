package admincfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// File is the demo admin app config shape this interface may rewrite.
type File struct {
	CustomerID string `json:"customer_id"`
	HMACSecret string `json:"hmac_secret"`
}

// Write creates or overwrites the admin config JSON (0600).
func Write(path, customerID, secret string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(File{
		CustomerID: customerID,
		HMACSecret: secret,
	}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
