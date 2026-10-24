package ui

import (
	"fmt"
	"math"
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
			fyne.Do(func() {
				n.isUpdating = true
				n.Entry.SetText(filtered)
				n.isUpdating = false
			})
		}
	}

	// Commit action (button click or Enter key)
	commit := func() {
		fyne.Do(func() {
			v, err := strconv.Atoi(n.Entry.Text)
			if err != nil || v < n.Min {
				v = n.Min
			} else if v > n.Max {
				v = n.Max
			}
			n.Value = v
			strVal := strconv.Itoa(v)
			if n.Entry.Text != strVal {
				n.isUpdating = true
				n.Entry.SetText(strVal)
				n.isUpdating = false
			}

			if n.OnApply != nil {
				n.OnApply(v)
			}
		})
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
	strVal := strconv.Itoa(v)
	if n.Entry.Text != strVal {
		n.isUpdating = true
		n.Entry.SetText(strVal)
		n.isUpdating = false
	}
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

// FloatCommitInput is a reusable UI component that provides a decimal numeric text entry
// paired with an "OK" commit button and Enter key support, with min/max bounds enforcement.
type FloatCommitInput struct {
	Container *fyne.Container
	Entry     *widget.Entry
	Button    *widget.Button
	Min       float64
	Max       float64
	Value     float64
	OnApply   func(val float64)

	isUpdating bool
}

func formatFloatSec(v float64) string {
	rounded := math.Round(v*100) / 100
	if math.Abs(rounded*10-math.Round(rounded*10)) < 1e-6 {
		return fmt.Sprintf("%.1f", rounded)
	}
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

// NewFloatCommitInput creates a new decimal numeric entry with an "OK" button and bounds checking.
func NewFloatCommitInput(initialVal, minVal, maxVal float64, prefixLabel string, onApply func(float64)) *FloatCommitInput {
	f := &FloatCommitInput{
		Min:     minVal,
		Max:     maxVal,
		Value:   initialVal,
		OnApply: onApply,
	}

	f.Entry = widget.NewEntry()
	f.Entry.SetText(formatFloatSec(initialVal))
	f.Entry.SetPlaceHolder(fmt.Sprintf("%s-%s", formatFloatSec(minVal), formatFloatSec(maxVal)))

	f.Entry.OnChanged = func(s string) {
		if f.isUpdating {
			return
		}
		var clean []rune
		hasDot := false
		for _, r := range s {
			if unicode.IsDigit(r) {
				clean = append(clean, r)
			} else if r == '.' && !hasDot {
				hasDot = true
				clean = append(clean, r)
			}
		}
		filtered := string(clean)
		if filtered != s {
			fyne.Do(func() {
				f.isUpdating = true
				f.Entry.SetText(filtered)
				f.isUpdating = false
			})
		}
	}

	commit := func() {
		fyne.Do(func() {
			v, err := strconv.ParseFloat(f.Entry.Text, 64)
			if err != nil || v < f.Min {
				v = f.Min
			} else if v > f.Max {
				v = f.Max
			}
			v = math.Round(v*100) / 100
			f.Value = v
			strVal := formatFloatSec(v)
			if f.Entry.Text != strVal {
				f.isUpdating = true
				f.Entry.SetText(strVal)
				f.isUpdating = false
			}

			if f.OnApply != nil {
				f.OnApply(v)
			}
		})
	}

	f.Button = widget.NewButton("OK", commit)
	f.Button.Importance = widget.LowImportance

	f.Entry.OnSubmitted = func(_ string) {
		commit()
	}

	var row *fyne.Container
	if prefixLabel != "" {
		label := widget.NewLabel(prefixLabel)
		row = container.NewBorder(nil, nil, label, f.Button, f.Entry)
	} else {
		row = container.NewBorder(nil, nil, nil, f.Button, f.Entry)
	}

	f.Container = row
	return f
}

// SetValue updates the current value and text without triggering the OnApply callback.
func (f *FloatCommitInput) SetValue(v float64) {
	if v < f.Min {
		v = f.Min
	} else if v > f.Max {
		v = f.Max
	}
	v = math.Round(v*100) / 100
	f.Value = v
	strVal := formatFloatSec(v)
	if f.Entry.Text != strVal {
		f.isUpdating = true
		f.Entry.SetText(strVal)
		f.isUpdating = false
	}
}

// commitEntry is an Entry that triggers onCommit both when Enter is pressed (OnSubmitted)
// and when focus is lost (FocusLost).
type commitEntry struct {
	widget.Entry
	onCommit func()
}

func newCommitEntry(onCommit func()) *commitEntry {
	e := &commitEntry{onCommit: onCommit}
	e.ExtendBaseWidget(e)
	e.OnSubmitted = func(_ string) {
		if e.onCommit != nil {
			e.onCommit()
		}
	}
	return e
}

func (e *commitEntry) FocusLost() {
	e.Entry.FocusLost()
	if e.onCommit != nil {
		e.onCommit()
	}
}


