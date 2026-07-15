package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDeleteEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		wantID string
		wantOk bool
	}{
		{"valid", `{"id":"es_123_abc"}`, "es_123_abc", true},
		{"empty id", `{"id":""}`, "", false},
		{"missing id", `{}`, "", false},
		{"invalid json", `not json`, "", false},
		{"extra fields", `{"id":"es_1","x":9}`, "es_1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseDeleteEvent([]byte(tt.input))
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.wantID, got.ID)
			}
		})
	}
}

func TestParseCommentEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantID   string
		wantText string
		wantOk   bool
	}{
		{"valid", `{"id":"es_1","text":"hello"}`, "es_1", "hello", true},
		{"empty id", `{"id":"","text":"x"}`, "", "", false},
		{"missing text", `{"id":"es_1"}`, "", "", false},
		{"empty text", `{"id":"es_1","text":""}`, "", "", false},
		{"invalid json", `{broken`, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseCommentEvent([]byte(tt.input))
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.wantID, got.ID)
				assert.Equal(t, tt.wantText, got.Text)
			}
		})
	}
}
