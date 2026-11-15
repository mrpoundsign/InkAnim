package inksvg

import (
	"math"
	"strings"
	"testing"
)

func TestPhysicsFPSIndependence(t *testing.T) {
	svgXML := `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" viewBox="0 0 200 200">
  <g inkscape:groupmode="layer" id="layer1" inkscape:label="Scene">
    <g id="ball" inkscape:label="Ball">
      <circle cx="100" cy="30" r="10" fill="#38BDF8" />
    </g>
    <rect id="floor" x="10" y="160" width="180" height="10" inkscape:label="Floor Solid { bounce: 0.8; friction: 0.1 }" />
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgXML))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}
	doc.TotalDurationSec = 1.5
	doc.MotionPaths = append(doc.MotionPaths, MotionPath{
		ID:      "grav1",
		GroupID: "ball",
		Config: MotionConfig{
			Type:         "gravity",
			IsPhysics:    true,
			HasTimeRange: true,
			StartSec:     0.0,
			EndSec:       1.5,
			GravityScale: 1.0,
			Bounce:       0.8,
			Friction:     0.1,
		},
	})
	doc.PrecomputePhysics()

	// Sample at t = 0.6s under 10 FPS (frame 5 of 16, where 4/15 * 1.5 = 0.4s, or 6/15 * 1.5 = 0.6s)
	// and under 60 FPS (where 24/60 * 1.5 = 0.6s).
	poseA, okA := doc.InterpolatePhysicsPose("ball", 0.6)
	if !okA {
		t.Fatalf("expected precomputed physics pose for ball")
	}
	if poseA.DY <= 10.0 {
		t.Fatalf("expected ball to have fallen significantly at t=0.6s, got DY=%f", poseA.DY)
	}

	// Re-verify that changing doc.Frames (e.g. 10 FPS vs 60 FPS) produces identical InterpolatePhysicsPose at t=0.6s
	doc.Frames = make([]Frame, 15)
	pose10FPS, _ := doc.InterpolatePhysicsPose("ball", 0.6)
	doc.Frames = make([]Frame, 90)
	pose60FPS, _ := doc.InterpolatePhysicsPose("ball", 0.6)

	if math.Abs(pose10FPS.DX-pose60FPS.DX) > 1e-9 || math.Abs(pose10FPS.DY-pose60FPS.DY) > 1e-9 {
		t.Fatalf("expected 100%% FPS-independent pose at t=0.6s, got 10FPS=%+v vs 60FPS=%+v", pose10FPS, pose60FPS)
	}
}

func TestPhysicsGravityBounceOnSolidFloor(t *testing.T) {
	svgXML := `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" viewBox="0 0 200 200">
  <g inkscape:groupmode="layer" id="layer1" inkscape:label="Scene">
    <g id="ball" inkscape:label="Ball">
      <circle cx="100" cy="40" r="10" fill="#38BDF8" />
    </g>
    <line id="floor" x1="0" y1="150" x2="200" y2="150" stroke="#10B981" stroke-width="2" inkscape:label="Solid { bounce: 0.75; friction: 0.1 }" />
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgXML))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}
	doc.TotalDurationSec = 2.0
	doc.MotionPaths = append(doc.MotionPaths, MotionPath{
		ID:      "grav1",
		GroupID: "ball",
		Config: MotionConfig{
			Type:         "gravity",
			IsPhysics:    true,
			HasTimeRange: true,
			StartSec:     0.0,
			EndSec:       2.0,
			GravityScale: 1.0,
			Bounce:       0.75,
			Friction:     0.1,
		},
	})
	doc.PrecomputePhysics()

	samples := doc.PhysicsSamples["ball"]
	if len(samples) == 0 {
		t.Fatalf("expected physics samples for ball")
	}

	// Ball starts at cy=40 with r=10, floor is at y=150 -> max DY before floor penetration is ~100.
	maxDY := 0.0
	rebounded := false
	reachedNearFloor := false
	for _, s := range samples {
		if s.DY > maxDY {
			maxDY = s.DY
		}
		if s.DY >= 95.0 {
			reachedNearFloor = true
		}
		if reachedNearFloor && s.DY < maxDY-15.0 {
			rebounded = true
		}
	}

	if !reachedNearFloor {
		t.Fatalf("expected ball to reach floor (DY >= 95), maxDY=%f", maxDY)
	}
	if maxDY > 105.0 {
		t.Fatalf("expected solid floor at y=150 to stop ball around DY=100, but ball penetrated to maxDY=%f", maxDY)
	}
	if !rebounded {
		t.Fatalf("expected ball to bounce upward off the solid floor, maxDY=%f", maxDY)
	}
}

func TestPhysicsPushAndSlopeRoll(t *testing.T) {
	svgXML := `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" viewBox="0 0 300 300">
  <g inkscape:groupmode="layer" id="layer1" inkscape:label="Scene">
    <g id="wheel" inkscape:label="Wheel">
      <circle cx="50" cy="50" r="10" fill="#FB923C" />
    </g>
    <line id="ramp" x1="20" y1="70" x2="260" y2="190" stroke="#10B981" stroke-width="2" inkscape:label="Solid { bounce: 0.2; friction: 0.1 }" />
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgXML))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}
	doc.TotalDurationSec = 1.5
	doc.MotionPaths = append(doc.MotionPaths,
		MotionPath{
			ID:       "push1",
			GroupID:  "wheel",
			PathData: "M 50 50 L 80 50",
			Config: MotionConfig{
				Type:         "push",
				IsPhysics:    true,
				HasTimeRange: true,
				StartSec:     0.0,
				EndSec:       1.5,
				Power:        4.0,
				Bounce:       0.2,
				Friction:     0.1,
				IsImpulse:    true,
				Roll:         true,
			},
		},
		MotionPath{
			ID:      "grav1",
			GroupID: "wheel",
			Config: MotionConfig{
				Type:         "gravity",
				IsPhysics:    true,
				HasTimeRange: true,
				StartSec:     0.0,
				EndSec:       1.5,
				GravityScale: 1.0,
				Bounce:       0.2,
				Friction:     0.1,
				Roll:         true,
			},
		},
	)
	doc.PrecomputePhysics()

	poseEnd, ok := doc.InterpolatePhysicsPose("wheel", 1.2)
	if !ok {
		t.Fatalf("expected physics pose for wheel")
	}
	if poseEnd.DX <= 20.0 {
		t.Fatalf("expected wheel to move right from push + ramp slope, got DX=%f", poseEnd.DX)
	}
	if math.Abs(poseEnd.DRot) < 10.0 {
		t.Fatalf("expected wheel with Roll=true to accumulate rotation on slope, got DRot=%f", poseEnd.DRot)
	}
}

func TestPhysicsLayerCollisionScopingAndHideSolid(t *testing.T) {
	svgXML := `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" viewBox="0 0 200 200">
  <g inkscape:groupmode="layer" id="layer1" inkscape:label="Foreground">
    <g id="ball" inkscape:label="Ball">
      <circle cx="100" cy="30" r="10" fill="#38BDF8" />
    </g>
  </g>
  <g inkscape:groupmode="layer" id="layer2" inkscape:label="Background">
    <rect id="bg_floor" x="0" y="80" width="200" height="10" fill="#10B981" />
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgXML))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}
	doc.TotalDurationSec = 1.0
	doc.StageObjects["bg_floor"] = StageObject{
		GroupID:       "bg_floor",
		IsSolid:       true,
		SolidBounce:   0.7,
		SolidFriction: 0.2,
		SolidGlobal:   false, // Scoped to layer2 only!
		HideSolid:     true,
	}
	doc.MotionPaths = append(doc.MotionPaths, MotionPath{
		ID:      "grav1",
		GroupID: "ball",
		Config: MotionConfig{
			Type:         "gravity",
			IsPhysics:    true,
			HasTimeRange: true,
			StartSec:     0.0,
			EndSec:       1.0,
			GravityScale: 1.0,
			Bounce:       0.7,
			Friction:     0.2,
		},
	})

	// 1. With SolidGlobal = false, ball in layer1 should NOT collide with bg_floor in layer2 (at y=80, DY=40)
	doc.PrecomputePhysics()
	poseScoped, _ := doc.InterpolatePhysicsPose("ball", 0.8)
	if poseScoped.DY <= 60.0 {
		t.Fatalf("expected ball in layer1 to ignore non-global solid in layer2 and fall past DY=60, got DY=%f", poseScoped.DY)
	}

	// 2. With SolidGlobal = true, ball in layer1 SHOULD collide with bg_floor at y=80 (max DY ~ 40)
	st := doc.StageObjects["bg_floor"]
	st.SolidGlobal = true
	doc.StageObjects["bg_floor"] = st
	doc.PrecomputePhysics()
	poseGlobal, _ := doc.InterpolatePhysicsPose("ball", 0.8)
	if poseGlobal.DY > 45.0 {
		t.Fatalf("expected ball to collide with SolidGlobal bg_floor around DY=40, got DY=%f", poseGlobal.DY)
	}

	// 3. Verify HideSolid hides bg_floor in generated frame XML
	frameBytes, err := BuildTimelineFrameSVGAt(doc, 1.0, Rect{})
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVGAt failed: %v", err)
	}
	frameStr := string(frameBytes)
	if !strings.Contains(frameStr, `id="bg_floor"`) {
		t.Fatalf("expected bg_floor element token in XML")
	}
	if !strings.Contains(frameStr, `display:none`) {
		t.Fatalf("expected HideSolid=true collider to have display:none in rendered frame XML")
	}
}
