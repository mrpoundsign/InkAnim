package params

import (
	"testing"
)

func TestParseAndFormat_Rot(t *testing.T) {
	// 1. Legacy angle migrated to deg
	raw := "f: 1-20; angle: 8; pingpong; r: 4"
	m := Parse("Rot", raw, 20)
	if m.StartFrame != 1 || m.EndFrame != 20 {
		t.Errorf("frames = %d-%d, want 1-20", m.StartFrame, m.EndFrame)
	}
	if m.Angle != 8 {
		t.Errorf("angle = %f, want 8", m.Angle)
	}
	if !m.PingPong {
		t.Errorf("pingpong = false, want true")
	}
	if m.Repeat != 4 {
		t.Errorf("repeat = %d, want 4", m.Repeat)
	}

	formatted := m.Format()
	if formatted != "f: 1-20; deg: 8; pingpong; r: 4" {
		t.Errorf("formatted = %q, want 'f: 1-20; deg: 8; pingpong; r: 4'", formatted)
	}

	// 2. Modern deg syntax preserved
	m2 := Parse("Rot", "f: 1-20; deg: 90", 20)
	if m2.Angle != 90 {
		t.Errorf("m2.Angle = %f, want 90", m2.Angle)
	}
	if m2.Format() != "f: 1-20; deg: 90" {
		t.Errorf("m2.Format() = %q, want 'f: 1-20; deg: 90'", m2.Format())
	}

	// 3. Legacy from/to migrated to deg delta
	m3 := Parse("Rot", "f: 13-24; from: 0; to: 180; pivot: center", 20)
	if m3.Angle != 180 {
		t.Errorf("m3.Angle = %f, want 180", m3.Angle)
	}
	if m3.Format() != "f: 13-24; deg: 180" {
		t.Errorf("m3.Format() = %q, want 'f: 13-24; deg: 180'", m3.Format())
	}

	m4 := Parse("Rot", "f: 37-48; from: 180; to: 0; pivot: center", 20)
	if m4.Angle != -180 {
		t.Errorf("m4.Angle = %f, want -180", m4.Angle)
	}
	if m4.Format() != "f: 37-48; deg: -180" {
		t.Errorf("m4.Format() = %q, want 'f: 37-48; deg: -180'", m4.Format())
	}
}

func TestParseAndFormat_Scale(t *testing.T) {
	raw := "f: 1-20; from: 1; to: 1.25; pingpong"
	m := Parse("Scale", raw, 20)
	if m.ScaleFrom != 1.0 || m.ScaleTo != 1.25 {
		t.Errorf("scale = %f to %f, want 1.0 to 1.25", m.ScaleFrom, m.ScaleTo)
	}
	if !m.PingPong {
		t.Errorf("pingpong = false, want true")
	}

	formatted := m.Format()
	if formatted != "f: 1-20; from: 1; to: 1.25; pingpong" {
		t.Errorf("formatted = %q, want 'f: 1-20; from: 1; to: 1.25; pingpong'", formatted)
	}
}

func TestParseAndFormat_Move(t *testing.T) {
	raw := "f: 1-20; ease: in-out; pingpong; orient"
	m := Parse("Move", raw, 20)
	if m.Ease != "in-out" {
		t.Errorf("ease = %q, want 'in-out'", m.Ease)
	}
	if !m.PingPong {
		t.Errorf("pingpong = false, want true")
	}
	if !m.Orient {
		t.Errorf("orient = false, want true")
	}

	formatted := m.Format()
	if formatted != "f: 1-20; orient; ease: in-out; pingpong" {
		t.Errorf("formatted = %q", formatted)
	}
}

func TestParseAndFormat_Fade(t *testing.T) {
	raw := "f: 1-15; from: 0; to: 1"
	m := Parse("Fade", raw, 20)
	if m.OpacityFrom != 0.0 || m.OpacityTo != 1.0 {
		t.Errorf("opacity = %f to %f, want 0 to 1", m.OpacityFrom, m.OpacityTo)
	}
	formatted := m.Format()
	if formatted != "f: 1-15; from: 0; to: 1" {
		t.Errorf("formatted = %q", formatted)
	}
}
