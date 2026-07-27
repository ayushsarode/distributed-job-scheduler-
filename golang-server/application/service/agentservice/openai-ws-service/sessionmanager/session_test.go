package sessionmanager

import (
	"sync"
	"testing"

	"exiro.ai/application/service/types"
	"github.com/rs/zerolog"
)

func TestSessionHandle_ConcurrentAccess(t *testing.T) {
	logger := zerolog.Nop()
	// Create a session with a nil conn - we only test state mutations
	s := newSession(nil, "model", "instr", "agent1")
	handle := &SessionHandle{sessionID: "s1", session: s, logger: &logger}

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				handle.SetLanguage(types.AgentLanguageEnglish)
			} else {
				handle.SetLanguage(types.AgentLanguageHindi)
			}
			handle.SetPrevResponseID("resp")
			handle.SetPendingCutCall(n%3 == 0, "msg")
		}(i)
	}
	wg.Wait()

	// Should not panic; final state is arbitrary
	_ = handle.Language()
	_ = handle.PrevResponseID()
	_ = handle.PendingCutCall()
	_ = handle.CutCallMessage()
}
