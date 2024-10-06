package event

import "testing"

func TestOnceEvent(t *testing.T) {
	oe := NewOnceEvent()
	if oe.HasFired() {
		t.Errorf("HasFired error")
	}
	if !oe.Trigger() {
		t.Errorf("Trigger faill")
	}
	if !oe.HasFired() {
		t.Errorf("HasFired error")
	}
	<-oe.Done()
	t.Logf("oe done")
}
