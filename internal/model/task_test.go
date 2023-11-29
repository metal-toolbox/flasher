package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskStatusRecord(t *testing.T) {
	tests := []struct {
		name           string
		appendStatus   string
		updateStatus   map[string]string
		appendStatuses []string
		wantStatuses   []string
	}{
		{
			"single status record appended",
			"works",
			nil,
			nil,
			[]string{"works"},
		},
		{
			"multiple status record appended",
			"",
			nil,
			[]string{"a", "b", "c"},
			[]string{"a", "b", "c"},
		},
		{
			"dup status excluded",
			"",
			nil,
			[]string{"a", "a", "b", "c"},
			[]string{"a", "b", "c"},
		},
		{
			"empty status excluded",
			"",
			nil,
			[]string{"a", "", "", "c"},
			[]string{"a", "c"},
		},
		{
			"truncates long set of statuses",
			"",
			nil,
			[]string{"a", "b", "c", "d", "e", "f"},
			[]string{"b", "c", "d", "e", "f"},
		},
		{
			"update a status record",
			"",
			map[string]string{"b": "updated", "d": "also updated"},
			[]string{"a", "b", "c", "d", "e"},
			[]string{"a", "updated", "c", "also updated", "e"},
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

			// continue with other tests when theres no appendStatuses
			if tc.appendStatuses == nil {
				return
			}

			for _, s := range tc.appendStatuses {
				sr.Append(s)
			}

			// append statuses - to test Update()
			if tc.updateStatus != nil {
				for k, v := range tc.updateStatus {
					sr.Update(k, v)
				}
			}

			assert.Equal(t, len(tc.wantStatuses), len(sr.StatusMsgs))
			for idx, w := range tc.wantStatuses {
				assert.Equal(t, w, sr.StatusMsgs[idx].Msg)
				assert.False(t, sr.StatusMsgs[idx].Timestamp.IsZero())
			}

			// test Last() method
			assert.Equal(t, tc.appendStatuses[len(tc.appendStatuses)-1], sr.Last())
		})
	}
}
