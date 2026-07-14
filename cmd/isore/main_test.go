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

func TestParseEditEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantID   string
		wantNote string
		wantOk   bool
	}{
		{"valid", `{"id":"es_1","note":"hello"}`, "es_1", "hello", true},
		{"empty id", `{"id":"","note":"x"}`, "", "", false},
		{"missing note", `{"id":"es_1"}`, "es_1", "", true},
		{"invalid json", `{broken`, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseEditEvent([]byte(tt.input))
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.wantID, got.ID)
				assert.Equal(t, tt.wantNote, got.Note)
			}
		})
	}
}
