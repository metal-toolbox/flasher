package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskStatusRecord(t *testing.T) {
	tests := []struct {
		name           string
		appendStatus   string
		appendStatuses []string
		wantStatuses   []string
	}{
		{
			"single status record appended",
			"works",
			nil,
			[]string{"works"},
		},
		{
			"multiple status record appended",
			"",
			[]string{"a", "b", "c"},
			[]string{"a", "b", "c"},
		},
		{
			"dup status excluded",
			"",
			[]string{"a", "a", "b", "c"},
			[]string{"a", "b", "c"},
		},
		{
			"empty status excluded",
			"",
			[]string{"a", "", "", "c"},
			[]string{"a", "c"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sr := NewTaskStatusRecord("")

			if tc.appendStatus != "" {
				sr.Append(tc.appendStatus)

				assert.Equal(t, tc.appendStatus, sr.StatusMsgs[0].Msg)
				assert.False(t, sr.StatusMsgs[0].Timestamp.IsZero())
			}

			if tc.appendStatuses != nil {
				for _, s := range tc.appendStatuses {
					sr.Append(s)
				}

				assert.Equal(t, len(tc.wantStatuses), len(sr.StatusMsgs))
				for idx, w := range tc.wantStatuses {
					assert.Equal(t, w, sr.StatusMsgs[idx].Msg)
					assert.False(t, sr.StatusMsgs[idx].Timestamp.IsZero())
				}
			}
		})
	}
}
