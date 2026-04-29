package cockpit

import (
	"encoding/json"
	"os"
	"time"
)

func loadForemanState(paths Paths) ForemanState {
	b, err := os.ReadFile(paths.ForemanFile)
	if err != nil {
		return ForemanState{}
	}
	var st ForemanState
	if err := json.Unmarshal(b, &st); err != nil {
		return ForemanState{}
	}
	return st
}

func saveForemanState(paths Paths, st ForemanState) error {
	if st.UpdatedAt.IsZero() {
		st.UpdatedAt = time.Now()
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ForemanFile, append(b, '\n'), 0o644)
}
