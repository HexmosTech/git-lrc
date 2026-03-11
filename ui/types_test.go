package ui

import "testing"

func TestConnectorSectionConstants(t *testing.T) {
	if AIConnectorsSectionBegin == "" || AIConnectorsSectionEnd == "" {
		t.Fatalf("connector section constants must not be empty")
	}
	if AIConnectorsSectionBegin == AIConnectorsSectionEnd {
		t.Fatalf("section markers must be distinct")
	}
}
