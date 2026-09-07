package ui

import (
	"fmt"
	"strconv"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// NumericCommitInput is a reusable UI component that provides a numeric-only text entry
// paired with an "OK" commit button and Enter key support, with min/max bounds enforcement.
type NumericCommitInput struct {
	Container *fyne.Container
	Entry     *widget.Entry
	Button    *widget.Button
	Min       int
	Max       int
	Value     int
	OnApply   func(val int)

	isUpdating bool
}

// NewNumericCommitInput creates a new numeric entry with an "OK" button and bounds checking.
func NewNumericCommitInput(initialVal, minVal, maxVal int, prefixLabel string, onApply func(int)) *NumericCommitInput {
	n := &NumericCommitInput{
		Min:     minVal,
		Max:     maxVal,
		Value:   initialVal,
		OnApply: onApply,
	}

	n.Entry = widget.NewEntry()
	n.Entry.SetText(strconv.Itoa(initialVal))
	n.Entry.SetPlaceHolder(fmt.Sprintf("%d-%d", minVal, maxVal))

	// Digits-only filtering on every change
	n.Entry.OnChanged = func(s string) {
		if n.isUpdating {
			return
		}
		var clean []rune
		for _, r := range s {
			if unicode.IsDigit(r) {
				clean = append(clean, r)
			}
		}
		filtered := string(clean)
		if filtered != s {
			n.isUpdating = true
			n.Entry.SetText(filtered)
			n.isUpdating = false
		}
	}

	// Commit action (button click or Enter key)
	commit := func() {
		v, err := strconv.Atoi(n.Entry.Text)
		if err != nil || v < n.Min {
			v = n.Min
		} else if v > n.Max {
			v = n.Max
		}
		n.Value = v
		n.isUpdating = true
		n.Entry.SetText(strconv.Itoa(v))
		n.isUpdating = false

		if n.OnApply != nil {
			n.OnApply(v)
		}
	}

	n.Button = widget.NewButton("OK", commit)
	n.Button.Importance = widget.LowImportance

	n.Entry.OnSubmitted = func(_ string) {
		commit()
	}

	var row *fyne.Container
	if prefixLabel != "" {
		label := widget.NewLabel(prefixLabel)
		row = container.NewBorder(nil, nil, label, n.Button, n.Entry)
	} else {
		row = container.NewBorder(nil, nil, nil, n.Button, n.Entry)
	}

	n.Container = row
	return n
}

// SetValue updates the current value and text without triggering the OnApply callback.
func (n *NumericCommitInput) SetValue(v int) {
	if v < n.Min {
		v = n.Min
	} else if v > n.Max {
		v = n.Max
	}
	n.Value = v
	n.isUpdating = true
	n.Entry.SetText(strconv.Itoa(v))
	n.isUpdating = false
}

// Show makes the input container visible.
func (n *NumericCommitInput) Show() {
	if n.Container != nil {
		n.Container.Show()
	}
}

// Hide hides the input container.
func (n *NumericCommitInput) Hide() {
	if n.Container != nil {
		n.Container.Hide()
	}
}

// Visible returns whether the input container is currently visible.
func (n *NumericCommitInput) Visible() bool {
	if n.Container != nil {
		return n.Container.Visible()
	}
	return false
}
