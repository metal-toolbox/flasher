package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrepend(t *testing.T) {
	a := &Action{ID: "a"}
	b := &Action{ID: "b"}
	c := &Action{ID: "c"}
	d := &Action{ID: "d"}

	want := Actions{a, b, c, d}

	actions := Actions{b, c, d}
	got := actions.Prepend(a)
	assert.Equal(t, want, got)
}
