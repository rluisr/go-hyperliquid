package hyperliquid

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"time"
)

type Exchange struct {
	client       *Client
	privateKey   *ecdsa.PrivateKey
	vault        string
	accountAddr  string
	info         *Info
	expiresAfter *int64
}

func NewExchange(
	privateKey *ecdsa.PrivateKey,
	baseURL string,
	meta *Meta,
	vaultAddr, accountAddr string,
	spotMeta *SpotMeta,
) *Exchange {
	return &Exchange{
		client:      NewClient(baseURL),
		privateKey:  privateKey,
		vault:       vaultAddr,
		accountAddr: accountAddr,
		info:        NewInfo(baseURL, true, meta, spotMeta),
	}
}

// executeAction executes an action and unmarshals the response into the given result
func (e *Exchange) executeAction(action any, result any) error {
	timestamp := time.Now().UnixMilli()

	sig, err := SignL1Action(
		e.privateKey,
		action,
		e.vault,
		timestamp,
		e.expiresAfter,
		e.client.baseURL == MainnetAPIURL,
	)
	if err != nil {
		return err
	}

	resp, err := e.postAction(action, sig, timestamp)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(resp, result); err != nil {
		return err
	}

	return nil
}

func (e *Exchange) postAction(
	action any,
	signature SignatureResult,
	nonce int64,
) ([]byte, error) {
	payload := map[string]any{
		"action":    action,
		"nonce":     nonce,
		"signature": signature,
	}
	
	// Debug: Print payload structure for trigger orders
	if actionStruct, ok := action.(OrderAction); ok {
		for i, order := range actionStruct.Orders {
			if order.OrderType.Trigger != nil {
				fmt.Printf("DEBUG: Trigger Order #%d - Asset: %d, IsBuy: %t, Vault: '%s'\n", i, order.Asset, order.IsBuy, e.vault)
			}
		}
	}

	// Always handle vault address to match signing logic
	// Handle vault address based on action type
	if actionMap, ok := action.(map[string]any); ok {
		if actionMap["type"] != "usdClassTransfer" {
			if e.vault != "" {
				payload["vaultAddress"] = e.vault
			}
			// Note: For empty vault, we don't include vaultAddress field
			// This matches the signing logic where empty vault adds 0x00 byte
		} else {
			payload["vaultAddress"] = nil
		}
	} else {
		// For struct types (like OrderAction) - including trigger orders
		// Always include vaultAddress to match authentication expectations
		if e.vault != "" {
			payload["vaultAddress"] = e.vault
		} else {
			// For empty vault, completely omit vaultAddress field for trigger orders
			if actionStruct, ok := action.(OrderAction); ok {
				hasTriggerOrder := false
				for _, order := range actionStruct.Orders {
					if order.OrderType.Trigger != nil {
						hasTriggerOrder = true
						break
					}
				}
				if hasTriggerOrder {
					// Don't add vaultAddress field at all for trigger orders with empty vault
					fmt.Printf("DEBUG: Omitting vaultAddress field completely for trigger order\n")
				}
			}
		}
	}

	// Add expiration time if set
	if e.expiresAfter != nil {
		payload["expiresAfter"] = *e.expiresAfter
	}

	return e.client.post("/exchange", payload)
}
