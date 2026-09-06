package gif

import (
	"fmt"
)

// TwitchValidationResult holds the result of validating an animation against Twitch guidelines.
type TwitchValidationResult struct {
	IsValid  bool     `json:"isValid"`
	Warnings []string `json:"warnings"`
	Errors   []string `json:"errors"`
}

// ValidateTwitchEmote validates frames and options against Twitch animated emote requirements.
func ValidateTwitchEmote(frameCount int, totalDurationMs int, width, height int, estimatedSizeBytes int64) TwitchValidationResult {
	res := TwitchValidationResult{
		IsValid: true,
	}

	// 1. Square aspect ratio
	if width != height {
		res.Errors = append(res.Errors, fmt.Sprintf("Emote must be square (1:1 aspect ratio). Current: %dx%d", width, height))
		res.IsValid = false
	}

	// 2. Resolution limits (between 112x112 and 4096x4096)
	if width < 112 || height < 112 {
		res.Errors = append(res.Errors, fmt.Sprintf("Resolution (%dx%d) is below Twitch minimum of 112x112", width, height))
		res.IsValid = false
	}
	if width > 4096 || height > 4096 {
		res.Errors = append(res.Errors, fmt.Sprintf("Resolution (%dx%d) exceeds Twitch maximum of 4096x4096", width, height))
		res.IsValid = false
	}

	// 3. File size limit (< 1MB = 1048576 bytes)
	if estimatedSizeBytes > 1048576 {
		res.Errors = append(res.Errors, fmt.Sprintf("File size (%0.2f MB) exceeds Twitch 1.0 MB limit", float64(estimatedSizeBytes)/(1024*1024)))
		res.IsValid = false
	} else if estimatedSizeBytes > 850000 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("File size (%0.2f MB) is close to the 1.0 MB limit", float64(estimatedSizeBytes)/(1024*1024)))
	}

	// 4. Frame count (max 60 frames)
	if frameCount > 60 {
		res.Errors = append(res.Errors, fmt.Sprintf("Frame count (%d) exceeds Twitch maximum of 60 frames", frameCount))
		res.IsValid = false
	} else if frameCount > 45 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("High frame count (%d frames); keep file size under 1MB", frameCount))
	}

	// 5. Total animation duration (recommended <= 3000 ms)
	if totalDurationMs > 3000 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("Duration (%0.1fs) exceeds Twitch recommended limit of 3 seconds", float64(totalDurationMs)/1000))
	}

	return res
}
