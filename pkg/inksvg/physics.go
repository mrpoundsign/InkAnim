package inksvg

import (
	"bytes"
	"encoding/xml"
	"math"
	"slices"
	"strconv"
	"strings"
)

const (
	// PhysicsHz is the fixed internal simulation frequency in steps per second (120 Hz).
	// Integrating at a fixed clock in seconds ensures that physical trajectories, velocities,
	// and bounce heights are 100% identical across all preview and export frame rates (e.g. 10 FPS vs 60 FPS).
	PhysicsHz = 120.0
	// PhysicsDT is the fixed timestep duration in seconds (1/120s).
	PhysicsDT = 1.0 / PhysicsHz
	// DefaultGravityAccel is the standard downward gravitational acceleration in SVG user-space pixels/sec².
	DefaultGravityAccel = 980.0
	// DefaultPushMultiplier scales a drawn force vector's pixel length into velocity (px/s) for impulses.
	DefaultPushMultiplier = 5.0
	// RestingNormalVelThreshold is the normal impact speed (px/s) below which restitution is damped to zero
	// so objects settle cleanly onto surfaces without infinite micro-bouncing.
	RestingNormalVelThreshold = 22.0
)

// SegmentCollider represents a 2D line segment belonging to a Solid surface.
type SegmentCollider struct {
	X1, Y1 float64
	X2, Y2 float64
}

type physicsSolidGroup struct {
	ID            string
	LayerID       string
	Global        bool
	Bounce        float64
	Friction      float64
	BaseSegments  []SegmentCollider
	OrigBounds    Rect
	HasStageObj   bool
	StageObj      StageObject
	KinematicActs []MotionPath
}

type extractedShapeInfo struct {
	ID        string
	LayerID   string
	Ancestors []string
	IsGuide   bool
	Segments  []SegmentCollider
}

// InterpolatePhysicsPose returns the linearly interpolated PhysicsPose (DX, DY, DRot) for groupID at curTimeSec.
func (d *SVGDocument) InterpolatePhysicsPose(groupID string, curTimeSec float64) (PhysicsPose, bool) {
	if d == nil || groupID == "" {
		return PhysicsPose{}, false
	}
	d.mu.RLock()
	samples, ok := d.PhysicsSamples[groupID]
	d.mu.RUnlock()
	if !ok || len(samples) == 0 {
		return PhysicsPose{}, false
	}

	if curTimeSec <= 0 {
		return samples[0], true
	}
	fIdx := curTimeSec * PhysicsHz
	maxIdx := len(samples) - 1
	if fIdx >= float64(maxIdx) {
		return samples[maxIdx], true
	}
	i0 := int(math.Floor(fIdx))
	i1 := min(i0+1, maxIdx)
	alpha := fIdx - float64(i0)
	s0 := samples[i0]
	s1 := samples[i1]
	return PhysicsPose{
		DX:   s0.DX + alpha*(s1.DX-s0.DX),
		DY:   s0.DY + alpha*(s1.DY-s0.DY),
		DRot: s0.DRot + alpha*(s1.DRot-s0.DRot),
	}, true
}

// PrecomputePhysics runs the deterministic 120 Hz 2D arcade physics simulation across [0, TotalDurationSec]
// for all objects with Push or Gravity actions, resolving collisions against scoped Solid surfaces.
func (d *SVGDocument) PrecomputePhysics() {
	if d == nil || len(d.RawContent) == 0 {
		return
	}

	// Group all motion paths by GroupID and check whether any physics actions exist.
	motionByGroup := make(map[string][]MotionPath)
	var hasPhysics bool
	for _, mp := range d.MotionPaths {
		if mp.GroupID != "" {
			motionByGroup[mp.GroupID] = append(motionByGroup[mp.GroupID], mp)
		}
		if mp.Config.IsPhysics || mp.Config.Type == "push" || mp.Config.Type == "gravity" {
			hasPhysics = true
		}
	}

	if !hasPhysics {
		d.mu.Lock()
		d.PhysicsSamples = nil
		d.mu.Unlock()
		return
	}

	totalDurSec := d.TotalDurationSec
	maxF := max(effectiveMaxFrames(d), 2)
	if totalDurSec <= 0 {
		totalDurSec = float64(maxF) * 0.1
		if totalDurSec <= 0 {
			totalDurSec = 1.0
		}
	}

	shapes, layerByID, descendantsByID := extractPhysicsScene(d.RawContent)

	// Identify dynamic bodies (any group/element with Push or Gravity actions)
	var dynamicIDs []string
	dynamicSubtree := make(map[string]bool)
	for groupID, mps := range motionByGroup {
		isDyn := false
		for _, mp := range mps {
			if mp.Config.IsPhysics || mp.Config.Type == "push" || mp.Config.Type == "gravity" {
				isDyn = true
				break
			}
		}
		if isDyn {
			dynamicIDs = append(dynamicIDs, groupID)
			dynamicSubtree[groupID] = true
			for _, desc := range descendantsByID[groupID] {
				dynamicSubtree[desc] = true
			}
		}
	}

	if len(dynamicIDs) == 0 {
		d.mu.Lock()
		d.PhysicsSamples = nil
		d.mu.Unlock()
		return
	}

	// Build Solid Collider groups from StageObjects (IsSolid) and legacy Solid MotionPaths
	solidMap := make(map[string]*physicsSolidGroup)
	for id, stObj := range d.StageObjects {
		if !stObj.IsSolid {
			continue
		}
		solidMap[id] = &physicsSolidGroup{
			ID:            id,
			LayerID:       layerByID[id],
			Global:        stObj.SolidGlobal,
			Bounce:        stObj.SolidBounce,
			Friction:      stObj.SolidFriction,
			OrigBounds:    d.GetElementRect(id),
			HasStageObj:   true,
			StageObj:      stObj,
			KinematicActs: motionByGroup[id],
		}
	}
	for _, mp := range d.MotionPaths {
		if !mp.Config.IsSolid && mp.Config.Type != "solid" {
			continue
		}
		targetID := mp.ID
		if targetID == "" || len(descendantsByID[targetID]) == 0 {
			// Check if mp.ID matches a shape directly; if not, fall back to GroupID
			hasShape := false
			for _, sh := range shapes {
				if sh.ID == targetID {
					hasShape = true
					break
				}
			}
			if !hasShape && mp.GroupID != "" {
				targetID = mp.GroupID
			}
		}
		if targetID == "" {
			continue
		}
		if _, exists := solidMap[targetID]; !exists {
			stObj, hasSt := d.StageObjects[targetID]
			solidMap[targetID] = &physicsSolidGroup{
				ID:            targetID,
				LayerID:       layerByID[targetID],
				Global:        mp.Config.SolidGlobal,
				Bounce:        mp.Config.Bounce,
				Friction:      mp.Config.Friction,
				OrigBounds:    d.GetElementRect(targetID),
				HasStageObj:   hasSt,
				StageObj:      stObj,
				KinematicActs: motionByGroup[targetID],
			}
		}
	}

	// Populate geometry segments for each solid group
	for solidID, sg := range solidMap {
		for _, sh := range shapes {
			if dynamicSubtree[sh.ID] {
				continue
			}
			// Skip motion guide paths unless the guide path itself was explicitly marked as the solid target
			if sh.IsGuide && sh.ID != solidID {
				continue
			}
			matches := sh.ID == solidID
			if !matches {
				if slices.Contains(sh.Ancestors, solidID) {
					matches = true
				}
			}
			if matches && len(sh.Segments) > 0 {
				sg.BaseSegments = append(sg.BaseSegments, sh.Segments...)
			}
		}
		// Fallback to bounding box edges if no vector segments were extracted (e.g. <image> or <use>)
		if len(sg.BaseSegments) == 0 && (sg.OrigBounds.Width > 0 || sg.OrigBounds.Height > 0) {
			r := sg.OrigBounds
			sg.BaseSegments = []SegmentCollider{
				{X1: r.X, Y1: r.Y, X2: r.X + r.Width, Y2: r.Y},
				{X1: r.X + r.Width, Y1: r.Y, X2: r.X + r.Width, Y2: r.Y + r.Height},
				{X1: r.X + r.Width, Y1: r.Y + r.Height, X2: r.X, Y2: r.Y + r.Height},
				{X1: r.X, Y1: r.Y + r.Height, X2: r.X, Y2: r.Y},
			}
		}
	}

	numSamples := max(int(math.Ceil(totalDurSec*PhysicsHz))+1, 2)
	results := make(map[string][]PhysicsPose, len(dynamicIDs))

	for _, groupID := range dynamicIDs {
		results[groupID] = simulateBodyTrajectory(d, groupID, layerByID[groupID], motionByGroup[groupID], solidMap, numSamples, maxF, totalDurSec)
	}

	d.mu.Lock()
	d.PhysicsSamples = results
	d.mu.Unlock()
}

func simulateBodyTrajectory(
	doc *SVGDocument,
	groupID string,
	bodyLayerID string,
	actions []MotionPath,
	solidMap map[string]*physicsSolidGroup,
	numSamples int,
	maxF int,
	totalDurSec float64,
) []PhysicsPose {
	samples := make([]PhysicsPose, numSamples)

	nativeRect := doc.GetElementRect(groupID)
	w := nativeRect.Width
	h := nativeRect.Height
	if w <= 0 {
		w = 24
	}
	if h <= 0 {
		h = 24
	}

	stObj, hasStageObj := doc.StageObjects[groupID]
	if hasStageObj {
		if stObj.BaseScaleX > 0 {
			w *= stObj.BaseScaleX
		}
		if stObj.BaseScaleY > 0 {
			h *= stObj.BaseScaleY
		}
	}

	halfW := max(w/2.0, 2.0)
	halfH := max(h/2.0, 2.0)
	avgRadius := max((halfW+halfH)/2.0, 4.0)

	startX := nativeRect.X
	startY := nativeRect.Y
	if hasStageObj && stObj.OverrideStartPos {
		startX = stObj.StartX
		startY = stObj.StartY
	}
	startCX := startX + nativeRect.Width/2.0
	startCY := startY + nativeRect.Height/2.0

	currCX := startCX
	currCY := startCY
	var vx, vy float64
	var currRot float64

	impulseFired := make([]bool, len(actions))
	prevKinDX, prevKinDY := evalKinematicMoveOffset(actions, 0.0, maxF, totalDurSec)

	for step := range numSamples {
		tSec := float64(step) * PhysicsDT
		if tSec > totalDurSec {
			tSec = totalDurSec
		}

		// Record current pose at timestamp tSec before advancing to next step
		samples[step] = PhysicsPose{
			DX:   currCX - startCX,
			DY:   currCY - startCY,
			DRot: currRot,
		}
		if step == numSamples-1 {
			break
		}

		// Check if body is hidden by its Display Window prior to StartTimeSec
		if hasStageObj && stObj.HasDisplayWindow && tSec < stObj.StartTimeSec-1e-6 {
			continue
		}

		var ax, ay float64
		bodyBounce := 0.65
		bodyFriction := 0.2
		var hasBounceOverride, hasFrictionOverride bool
		var rollEnabled bool
		var physicsActiveAtStep bool

		kfPos := 1.0
		if totalDurSec > 0 && maxF > 1 {
			kfPos = 1.0 + (tSec/totalDurSec)*float64(maxF-1)
		}

		for i, mp := range actions {
			cfg := mp.Config
			if !cfg.IsPhysics && cfg.Type != "push" && cfg.Type != "gravity" {
				continue
			}
			inRange, beforeStart, afterEnd, _, _ := evalMotionTiming(cfg, kfPos, maxF, tSec, totalDurSec)

			// Once an impulse or gravity action has started, the body remains physically simulated
			if !beforeStart {
				physicsActiveAtStep = true
				if !hasBounceOverride {
					bodyBounce = cfg.Bounce
					hasBounceOverride = true
				}
				if !hasFrictionOverride {
					bodyFriction = cfg.Friction
					hasFrictionOverride = true
				}
				if cfg.Roll {
					rollEnabled = true
				}
			}

			switch cfg.Type {
			case "push":
				if cfg.IsImpulse {
					if !beforeStart && !impulseFired[i] {
						impulseFired[i] = true
						fx, fy := extractForceVector(mp.PathData, cfg.RotationAngle, cfg.Reverse, 80.0)
						power := cfg.Power
						if power <= 0 {
							power = DefaultPushMultiplier
						}
						vx += fx * power
						vy += fy * power
					}
				} else if inRange && !afterEnd {
					fx, fy := extractForceVector(mp.PathData, cfg.RotationAngle, cfg.Reverse, 80.0)
					power := cfg.Power
					if power <= 0 {
						power = DefaultPushMultiplier
					}
					ax += fx * power * 8.0
					ay += fy * power * 8.0
				}

			case "gravity":
				if inRange && !afterEnd {
					gScale := cfg.GravityScale
					if gScale == 0 {
						gScale = 1.0
					}
					if strings.TrimSpace(mp.PathData) != "" {
						fx, fy := extractForceVector(mp.PathData, 90.0, cfg.Reverse, 100.0)
						if math.Hypot(fx, fy) > 1e-3 {
							ax += (fx / 100.0) * DefaultGravityAccel * gScale
							ay += (fy / 100.0) * DefaultGravityAccel * gScale
						} else {
							ay += DefaultGravityAccel * gScale
						}
					} else {
						ay += DefaultGravityAccel * gScale
					}
				}
			}
		}

		if !physicsActiveAtStep {
			continue
		}

		nextTSec := min(float64(step+1)*PhysicsDT, totalDurSec)
		kinDX, kinDY := evalKinematicMoveOffset(actions, nextTSec, maxF, totalDurSec)
		dKinX := kinDX - prevKinDX
		dKinY := kinDY - prevKinDY
		prevKinDX, prevKinDY = kinDX, kinDY

		// Integrate velocity (Symplectic Euler)
		vx += ax * PhysicsDT
		vy += ay * PhysicsDT

		// Gather active colliders for this layer / global scope at tSec
		activeSegs, segMeta := collectActiveColliderSegments(solidMap, bodyLayerID, tSec, PhysicsDT, maxF, totalDurSec)

		// Adaptive sub-stepping to prevent tunneling at high speed
		disp := math.Hypot(vx*PhysicsDT+dKinX, vy*PhysicsDT+dKinY)
		minR := min(halfW, halfH)
		subSteps := 1
		if disp > minR*0.45 && minR > 0 {
			subSteps = min(max(int(math.Ceil(disp/(minR*0.45))), 2), 6)
		}
		subDT := PhysicsDT / float64(subSteps)

		for range subSteps {
			prevX := currCX + (kinDX - dKinX)
			prevY := currCY + (kinDY - dKinY)
			currCX += vx * subDT
			currCY += vy * subDT
			worldX := currCX + kinDX
			worldY := currCY + kinDY

			if len(activeSegs) > 0 {
				newWorldX, newWorldY, newVX, newVY, dRot := resolveBodyCollisions(
					prevX, prevY,
					worldX, worldY,
					vx, vy,
					halfW, halfH, avgRadius,
					bodyBounce, bodyFriction, rollEnabled,
					activeSegs, segMeta,
					subDT,
				)
				currCX = newWorldX - kinDX
				currCY = newWorldY - kinDY
				vx = newVX
				vy = newVY
				currRot += dRot
			}
		}
	}

	return samples
}

type colliderSegMeta struct {
	bounce   float64
	friction float64
	velX     float64
	velY     float64
}

func collectActiveColliderSegments(
	solidMap map[string]*physicsSolidGroup,
	bodyLayerID string,
	tSec, dt float64,
	maxF int,
	totalDurSec float64,
) ([]SegmentCollider, []colliderSegMeta) {
	var segs []SegmentCollider
	var meta []colliderSegMeta

	for _, sg := range solidMap {
		if len(sg.BaseSegments) == 0 {
			continue
		}
		// Layer scoping: collide if SolidGlobal is true, or if both share the same LayerID,
		// or if one of them is at root/layer level matching the other.
		if !sg.Global && sg.LayerID != "" && bodyLayerID != "" && sg.LayerID != bodyLayerID && sg.ID != bodyLayerID {
			continue
		}
		if !isSolidActiveAtTime(sg, tSec, maxF, totalDurSec) {
			continue
		}

		dx0, dy0 := evalSolidOffsetAtTime(sg, tSec, maxF, totalDurSec)
		dx1, dy1 := evalSolidOffsetAtTime(sg, min(tSec+dt, totalDurSec), maxF, totalDurSec)
		svx := (dx1 - dx0) / dt
		svy := (dy1 - dy0) / dt

		m := colliderSegMeta{
			bounce:   sg.Bounce,
			friction: sg.Friction,
			velX:     svx,
			velY:     svy,
		}

		for _, baseSeg := range sg.BaseSegments {
			segs = append(segs, SegmentCollider{
				X1: baseSeg.X1 + dx0,
				Y1: baseSeg.Y1 + dy0,
				X2: baseSeg.X2 + dx0,
				Y2: baseSeg.Y2 + dy0,
			})
			meta = append(meta, m)
		}
	}

	return segs, meta
}

func isSolidActiveAtTime(sg *physicsSolidGroup, tSec float64, maxF int, totalDurSec float64) bool {
	if sg.HasStageObj && sg.StageObj.HasDisplayWindow {
		if tSec < sg.StageObj.StartTimeSec-1e-6 {
			return false
		}
		if sg.StageObj.EndTimeSec > 0 && tSec > sg.StageObj.EndTimeSec+1e-6 {
			return false
		}
	}
	kfPos := 1.0
	if totalDurSec > 0 && maxF > 1 {
		kfPos = 1.0 + (tSec/totalDurSec)*float64(maxF-1)
	}
	var hasShow, isShown, isHidden bool
	for _, mp := range sg.KinematicActs {
		inRange, _, _, _, _ := evalMotionTiming(mp.Config, kfPos, maxF, tSec, totalDurSec)
		switch mp.Config.VisibilityState {
		case "show":
			hasShow = true
			if inRange {
				isShown = true
			}
		case "hide":
			if inRange {
				isHidden = true
			}
		}
		if (mp.Config.IsSolid || mp.Config.Type == "solid") && !mp.Config.IsAll {
			if !inRange {
				return false
			}
		}
	}
	if (hasShow && !isShown) || isHidden {
		return false
	}
	return true
}

func evalSolidOffsetAtTime(sg *physicsSolidGroup, tSec float64, maxF int, totalDurSec float64) (float64, float64) {
	var dx, dy float64
	if sg.HasStageObj && sg.StageObj.OverrideStartPos {
		if sg.OrigBounds.Width > 0 || sg.OrigBounds.Height > 0 {
			dx += sg.StageObj.StartX - sg.OrigBounds.X
			dy += sg.StageObj.StartY - sg.OrigBounds.Y
		} else {
			dx += sg.StageObj.StartX
			dy += sg.StageObj.StartY
		}
	}
	kdx, kdy := evalKinematicMoveOffset(sg.KinematicActs, tSec, maxF, totalDurSec)
	return dx + kdx, dy + kdy
}

func evalKinematicMoveOffset(actions []MotionPath, tSec float64, maxF int, totalDurSec float64) (float64, float64) {
	var totalDX, totalDY float64
	kfPos := 1.0
	if totalDurSec > 0 && maxF > 1 {
		kfPos = 1.0 + (tSec/totalDurSec)*float64(maxF-1)
	}
	for _, mp := range actions {
		if mp.Config.Type != "move" || mp.PathData == "" {
			continue
		}
		_, beforeStart, _, progress, _ := evalMotionTiming(mp.Config, kfPos, maxF, tSec, totalDurSec)
		if beforeStart {
			continue
		}
		r := max(mp.Config.Repeat, 1)
		cycleP := progress * float64(r)
		var fraction float64
		if progress >= 1.0 {
			fraction = 1.0
		} else {
			fraction = cycleP - math.Floor(cycleP)
		}
		t := ApplyEasing(fraction, mp.Config.Ease)
		if mp.Config.Reverse {
			t = 1.0 - t
		}
		if mp.Config.IsPingPong {
			if t <= 0.5 {
				t *= 2.0
			} else {
				t = (1.0 - t) * 2.0
			}
		}
		if dx, dy, err := EvaluatePathAt(mp.PathData, t); err == nil {
			totalDX += dx
			totalDY += dy
		}
	}
	return totalDX, totalDY
}

func resolveBodyCollisions(
	prevX, prevY,
	currX, currY,
	vx, vy,
	halfW, halfH, avgRadius,
	bodyBounce, bodyFriction float64,
	rollEnabled bool,
	segs []SegmentCollider,
	meta []colliderSegMeta,
	dt float64,
) (newX, newY, newVX, newVY, dRot float64) {
	newX, newY = currX, currY
	newVX, newVY = vx, vy

	supportRadius := func(nx, ny float64) float64 {
		if rollEnabled {
			return avgRadius
		}
		return math.Hypot(halfW*nx, halfH*ny)
	}

	for i, seg := range segs {
		sx := seg.X2 - seg.X1
		sy := seg.Y2 - seg.Y1
		segLen2 := sx*sx + sy*sy
		if segLen2 < 1e-8 {
			continue
		}

		// 1. Check if center trajectory (prev -> new) crossed directly through the segment
		if hit, ix, iy, nx, ny := segmentCrossesSegment(prevX, prevY, newX, newY, seg.X1, seg.Y1, seg.X2, seg.Y2); hit {
			r := supportRadius(nx, ny)
			newX = ix + nx*r
			newY = iy + ny*r
			newVX, newVY, dRot = applySurfaceImpulse(newVX, newVY, nx, ny, avgRadius, bodyBounce, bodyFriction, rollEnabled, meta[i], dt, dRot)
			continue
		}

		// 2. Closest point on segment to current center
		qx, qy := closestPointOnSegment(newX, newY, seg.X1, seg.Y1, seg.X2, seg.Y2)
		dx := newX - qx
		dy := newY - qy
		dist := math.Hypot(dx, dy)

		var nx, ny float64
		if dist > 1e-6 {
			nx = dx / dist
			ny = dy / dist
		} else {
			// Fallback to segment normal oriented toward prev position
			segLen := math.Sqrt(segLen2)
			nx = -sy / segLen
			ny = sx / segLen
			if (prevX-qx)*nx+(prevY-qy)*ny < 0 {
				nx = -nx
				ny = -ny
			}
		}

		r := supportRadius(nx, ny)
		if dist < r {
			penetration := r - dist
			newX += nx * penetration
			newY += ny * penetration
			newVX, newVY, dRot = applySurfaceImpulse(newVX, newVY, nx, ny, avgRadius, bodyBounce, bodyFriction, rollEnabled, meta[i], dt, dRot)
		}
	}

	return newX, newY, newVX, newVY, dRot
}

func applySurfaceImpulse(
	vx, vy,
	nx, ny,
	avgRadius,
	bodyBounce, bodyFriction float64,
	rollEnabled bool,
	m colliderSegMeta,
	dt float64,
	accumRot float64,
) (float64, float64, float64) {
	relVX := vx - m.velX
	relVY := vy - m.velY
	vn := relVX*nx + relVY*ny

	// Tangential component
	vtx := relVX - vn*nx
	vty := relVY - vn*ny

	if vn < 0 {
		// Combine object bounciness and surface bounciness.
		// Using geometric mean / product where default surface bounce is 0.7:
		restitution := clampPhysics(bodyBounce*m.bounce/0.7, 0.0, 1.5)
		if bodyBounce == 0 || m.bounce == 0 {
			restitution = 0
		}
		if -vn < RestingNormalVelThreshold {
			restitution = 0
		}
		vn = -vn * restitution

		mu := clampPhysics(max(bodyFriction, m.friction), 0.0, 1.0)
		// Apply friction damping to tangential velocity
		damp := max(0.0, 1.0-mu*min(1.0, dt*18.0+0.12))
		vtx *= damp
		vty *= damp
	}

	if rollEnabled && avgRadius > 1e-3 {
		// 2D cross product (n x vt) gives signed tangential rolling speed in SVG clockwise convention
		tangentialSpeed := nx*vty - ny*vtx
		accumRot += (tangentialSpeed * dt / avgRadius) * (180.0 / math.Pi)
	}

	outVX := m.velX + vn*nx + vtx
	outVY := m.velY + vn*ny + vty
	return outVX, outVY, accumRot
}

func closestPointOnSegment(px, py, x1, y1, x2, y2 float64) (float64, float64) {
	dx := x2 - x1
	dy := y2 - y1
	len2 := dx*dx + dy*dy
	if len2 < 1e-9 {
		return x1, y1
	}
	u := ((px-x1)*dx + (py-y1)*dy) / len2
	if u < 0 {
		u = 0
	} else if u > 1 {
		u = 1
	}
	return x1 + u*dx, y1 + u*dy
}

func segmentCrossesSegment(p1x, p1y, p2x, p2y, q1x, q1y, q2x, q2y float64) (hit bool, ix, iy, nx, ny float64) {
	rx := p2x - p1x
	ry := p2y - p1y
	sx := q2x - q1x
	sy := q2y - q1y

	rxs := rx*sy - ry*sx
	if math.Abs(rxs) < 1e-8 {
		return false, 0, 0, 0, 0
	}
	qpx := q1x - p1x
	qpy := q1y - p1y

	t := (qpx*sy - qpy*sx) / rxs
	u := (qpx*ry - qpy*rx) / rxs

	if t >= 0 && t <= 1 && u >= 0 && u <= 1 {
		ix = p1x + t*rx
		iy = p1y + t*ry
		segLen := math.Hypot(sx, sy)
		if segLen < 1e-8 {
			return false, 0, 0, 0, 0
		}
		nx = -sy / segLen
		ny = sx / segLen
		// Orient normal toward p1 (the side the body approached from)
		if (p1x-ix)*nx+(p1y-iy)*ny < 0 {
			nx = -nx
			ny = -ny
		}
		return true, ix, iy, nx, ny
	}
	return false, 0, 0, 0, 0
}

func extractForceVector(pathData string, fallbackAngleDeg float64, reverse bool, defaultLen float64) (float64, float64) {
	pathData = strings.TrimSpace(pathData)
	if pathData != "" {
		if pp, err := getParsedPath(pathData); err == nil && len(pp.Segments) > 0 {
			endX, endY := pp.Segments[len(pp.Segments)-1].EvaluateAt(1.0)
			dx := endX - pp.StartX
			dy := endY - pp.StartY
			if reverse {
				dx = -dx
				dy = -dy
			}
			return dx, dy
		}
	}
	rad := fallbackAngleDeg * math.Pi / 180.0
	dx := defaultLen * math.Cos(rad)
	dy := defaultLen * math.Sin(rad)
	if reverse {
		dx = -dx
		dy = -dy
	}
	return dx, dy
}

func clampPhysics(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// extractPhysicsScene scans the SVG XML stream once to map every element ID to its enclosing layer,
// record parent-child descendant chains, and flatten drawable shapes into world-space line segments.
func extractPhysicsScene(rawContent []byte) (shapes []extractedShapeInfo, layerByID map[string]string, descendantsByID map[string][]string) {
	layerByID = make(map[string]string)
	descendantsByID = make(map[string][]string)

	type groupFrame struct {
		id       string
		isLayer  bool
		isSource bool
		matrix   Matrix2D
	}

	decoder := xml.NewDecoder(bytes.NewReader(rawContent))
	stack := []groupFrame{{id: "__root__", matrix: IdentityMatrix()}}

	currentLayerID := func() string {
		for _, s := range slices.Backward(stack) {
			if s.isLayer && s.id != "" {
				return s.id
			}
		}
		for _, s := range slices.Backward(stack) {
			if s.isSource && s.id != "" {
				return s.id
			}
		}
		return "__root__"
	}

	currentAncestors := func() []string {
		var ancs []string
		for _, f := range stack {
			if f.id != "" && f.id != "__root__" {
				ancs = append(ancs, f.id)
			}
		}
		return ancs
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "defs", "clipPath", "mask", "marker", "pattern", "filter", "linearGradient", "radialGradient", "namedview", "metadata", "style", "script":
				_ = decoder.Skip()
				continue
			}

			parentMat := stack[len(stack)-1].matrix
			localMat := IdentityMatrix()
			if tr := extractAttr(elem.Attr, "transform"); tr != "" {
				localMat = parseTransform(tr)
			}
			worldMat := parentMat.Multiply(localMat)

			id := extractAttr(elem.Attr, "id")
			rawLabel := extractAttr(elem.Attr, "label")
			groupMode := extractAttr(elem.Attr, "groupmode")
			sourceAttr := extractAttr(elem.Attr, "data-inka-source")

			if elem.Name.Local == "g" {
				isLayer := groupMode == "layer"
				isSource := sourceAttr != ""
				ancs := currentAncestors()
				if id != "" {
					for _, a := range ancs {
						descendantsByID[a] = append(descendantsByID[a], id)
					}
					if isLayer {
						layerByID[id] = id
					} else {
						layerByID[id] = currentLayerID()
					}
				}
				stack = append(stack, groupFrame{
					id:       id,
					isLayer:  isLayer,
					isSource: isSource,
					matrix:   worldMat,
				})
				continue
			}

			if isMotionShapeElement(elem.Name.Local) {
				if strings.HasSuffix(id, "_stroke") || id == "inkanim_page_background" {
					continue
				}
				ancs := currentAncestors()
				layID := currentLayerID()
				if id != "" {
					layerByID[id] = layID
					for _, a := range ancs {
						descendantsByID[a] = append(descendantsByID[a], id)
					}
				}
				var isGuide bool
				if strings.Contains(rawLabel, "{") {
					if cfg, ok := parseMotionConfig(rawLabel); ok && !cfg.IsSolid && cfg.Type != "solid" {
						isGuide = true
					}
				}
				segs := extractShapeSegments(elem, worldMat)
				if len(segs) > 0 {
					shapes = append(shapes, extractedShapeInfo{
						ID:        id,
						LayerID:   layID,
						Ancestors: ancs,
						IsGuide:   isGuide,
						Segments:  segs,
					})
				}
			}

		case xml.EndElement:
			if elem.Name.Local == "g" && len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	return shapes, layerByID, descendantsByID
}

func extractShapeSegments(elem xml.StartElement, m Matrix2D) []SegmentCollider {
	var segs []SegmentCollider
	addSeg := func(x1, y1, x2, y2 float64) {
		tx1, ty1 := m.Transform(x1, y1)
		tx2, ty2 := m.Transform(x2, y2)
		if math.Hypot(tx2-tx1, ty2-ty1) > 1e-4 {
			segs = append(segs, SegmentCollider{X1: tx1, Y1: ty1, X2: tx2, Y2: ty2})
		}
	}

	switch elem.Name.Local {
	case "line":
		x1 := parseDimension(extractAttr(elem.Attr, "x1"))
		y1 := parseDimension(extractAttr(elem.Attr, "y1"))
		x2 := parseDimension(extractAttr(elem.Attr, "x2"))
		y2 := parseDimension(extractAttr(elem.Attr, "y2"))
		addSeg(x1, y1, x2, y2)

	case "rect":
		x := parseDimension(extractAttr(elem.Attr, "x"))
		y := parseDimension(extractAttr(elem.Attr, "y"))
		w := parseDimension(extractAttr(elem.Attr, "width"))
		h := parseDimension(extractAttr(elem.Attr, "height"))
		if w > 0 && h > 0 {
			addSeg(x, y, x+w, y)
			addSeg(x+w, y, x+w, y+h)
			addSeg(x+w, y+h, x, y+h)
			addSeg(x, y+h, x, y)
		}

	case "circle":
		cx := parseDimension(extractAttr(elem.Attr, "cx"))
		cy := parseDimension(extractAttr(elem.Attr, "cy"))
		r := parseDimension(extractAttr(elem.Attr, "r"))
		if r > 0 {
			const steps = 20
			for i := range steps {
				a0 := float64(i) * 2.0 * math.Pi / float64(steps)
				a1 := float64(i+1) * 2.0 * math.Pi / float64(steps)
				addSeg(cx+r*math.Cos(a0), cy+r*math.Sin(a0), cx+r*math.Cos(a1), cy+r*math.Sin(a1))
			}
		}

	case "ellipse":
		cx := parseDimension(extractAttr(elem.Attr, "cx"))
		cy := parseDimension(extractAttr(elem.Attr, "cy"))
		rx := parseDimension(extractAttr(elem.Attr, "rx"))
		ry := parseDimension(extractAttr(elem.Attr, "ry"))
		if rx > 0 && ry > 0 {
			const steps = 20
			for i := range steps {
				a0 := float64(i) * 2.0 * math.Pi / float64(steps)
				a1 := float64(i+1) * 2.0 * math.Pi / float64(steps)
				addSeg(cx+rx*math.Cos(a0), cy+ry*math.Sin(a0), cx+rx*math.Cos(a1), cy+ry*math.Sin(a1))
			}
		}

	case "polygon", "polyline":
		pts := parsePointsList(extractAttr(elem.Attr, "points"))
		for i := 0; i+1 < len(pts); i++ {
			addSeg(pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1])
		}
		if elem.Name.Local == "polygon" && len(pts) >= 3 {
			addSeg(pts[len(pts)-1][0], pts[len(pts)-1][1], pts[0][0], pts[0][1])
		}

	case "path":
		d := extractAttr(elem.Attr, "d")
		if d == "" {
			return nil
		}
		pathSegs, _, _, err := parsePathSegments(d)
		if err != nil {
			return nil
		}
		for _, ps := range pathSegs {
			switch s := ps.(type) {
			case *LineSegment:
				addSeg(s.startX, s.startY, s.endX, s.endY)
			default:
				const curveSteps = 12
				px, py := ps.EvaluateAt(0)
				for k := 1; k <= curveSteps; k++ {
					u := float64(k) / float64(curveSteps)
					nx, ny := ps.EvaluateAt(u)
					addSeg(px, py, nx, ny)
					px, py = nx, ny
				}
			}
		}
	}

	return segs
}

func parsePointsList(ptsStr string) [][2]float64 {
	fields := strings.FieldsFunc(ptsStr, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	var pts [][2]float64
	for i := 0; i+1 < len(fields); i += 2 {
		x, err1 := strconv.ParseFloat(fields[i], 64)
		y, err2 := strconv.ParseFloat(fields[i+1], 64)
		if err1 == nil && err2 == nil {
			pts = append(pts, [2]float64{x, y})
		}
	}
	return pts
}
