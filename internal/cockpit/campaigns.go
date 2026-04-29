package cockpit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	lastCampaignID int64
	campaignIDMu   sync.Mutex
)

func NewCampaignID() CampaignID {
	campaignIDMu.Lock()
	defer campaignIDMu.Unlock()
	now := time.Now().UnixMilli()
	if now <= lastCampaignID {
		now = lastCampaignID + 1
	}
	lastCampaignID = now
	return CampaignID(fmt.Sprintf("c-%d", now))
}

func SaveCampaign(dir string, c Campaign) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, string(c.ID)+".json"), append(b, '\n'), 0o644)
}
