package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/browser"
	"github.com/pyneda/sukyan/pkg/browser/actions"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	recordURL         string
	recordActionsFile string
	recordActionsID   uint
	recordOutput      string
	recordFormat      string
	recordFPS         int
	recordQuality     int
	recordWidth       int
	recordHeight      int
	recordTimeout     time.Duration
	recordKeepFrames  bool
	recordDialogDelay time.Duration
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a browser session to video",
	Long: `Record browser actions to a video file for proof of concept demonstrations.

Examples:
  # Record navigating to a URL
  sukyan record --url https://example.com --output poc.avi

  # Record with browser actions from JSON file
  sukyan record --url https://example.com --actions actions.json --output poc.mp4

  # Record using stored browser actions by ID
  sukyan record --actions-id 5 --output poc.avi

  # Full options
  sukyan record --url https://example.com --actions actions.json \
    --output poc.mp4 --format h264 --fps 30 --quality 80 --timeout 60s`,
	RunE: runRecord,
}

func init() {
	rootCmd.AddCommand(recordCmd)

	recordCmd.Flags().StringVarP(&recordURL, "url", "u", "", "URL to navigate to")
	recordCmd.Flags().StringVarP(&recordActionsFile, "actions", "a", "", "Path to browser actions JSON file")
	recordCmd.Flags().UintVar(&recordActionsID, "actions-id", 0, "Stored browser actions ID to replay")
	recordCmd.Flags().StringVarP(&recordOutput, "output", "o", "", "Output video file path (required)")
	recordCmd.Flags().StringVarP(&recordFormat, "format", "f", "auto", "Video format: auto, mjpeg, or h264")
	recordCmd.Flags().IntVar(&recordFPS, "fps", 5, "Video framerate (lower = longer video, default 5 for PoC)")
	recordCmd.Flags().IntVar(&recordQuality, "quality", 80, "JPEG quality (1-100)")
	recordCmd.Flags().IntVar(&recordWidth, "width", 0, "Max viewport width (0 for default)")
	recordCmd.Flags().IntVar(&recordHeight, "height", 0, "Max viewport height (0 for default)")
	recordCmd.Flags().DurationVar(&recordTimeout, "timeout", 2*time.Minute, "Max recording duration")
	recordCmd.Flags().BoolVar(&recordKeepFrames, "keep-frames", false, "Keep frame images after encoding")
	recordCmd.Flags().DurationVar(&recordDialogDelay, "dialog-delay", 1500*time.Millisecond, "Delay before auto-dismissing JS dialogs (0 to disable)")
	recordCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID for task tracking")

	recordCmd.MarkFlagRequired("output")
}

func runRecord(cmd *cobra.Command, args []string) error {
	// Validate inputs
	if recordURL == "" && recordActionsID == 0 && recordActionsFile == "" {
		return fmt.Errorf("at least one of --url, --actions, or --actions-id is required")
	}

	// Parse video format
	var videoFormat browser.VideoOutputFormat
	switch recordFormat {
	case "auto":
		videoFormat = browser.VideoFormatAuto
	case "mjpeg", "avi":
		videoFormat = browser.VideoFormatMJPEG
	case "h264", "mp4":
		videoFormat = browser.VideoFormatH264
	default:
		return fmt.Errorf("invalid format %q: must be auto, mjpeg, or h264", recordFormat)
	}

	// Check ffmpeg if h264 explicitly requested
	if videoFormat == browser.VideoFormatH264 && !browser.IsFFmpegAvailable() {
		return fmt.Errorf("h264 format requires ffmpeg which is not installed; use --format=mjpeg or --format=auto")
	}

	// Collect browser actions
	var browserActions []actions.Action

	if recordActionsID != 0 {
		// Load actions from stored browser actions
		storedActions, err := db.Connection().GetStoredBrowserActionsByID(recordActionsID)
		if err != nil {
			return fmt.Errorf("failed to get stored browser actions %d: %w", recordActionsID, err)
		}

		browserActions = storedActions.Actions
		if recordURL == "" && len(browserActions) > 0 && browserActions[0].Type == actions.ActionNavigate {
			recordURL = browserActions[0].URL
		}

		log.Info().
			Uint("actions_id", recordActionsID).
			Str("title", storedActions.Title).
			Int("actions", len(browserActions)).
			Msg("Loaded stored browser actions")
	} else if recordActionsFile != "" {
		// Load actions from JSON file
		data, err := os.ReadFile(recordActionsFile)
		if err != nil {
			return fmt.Errorf("failed to read actions file: %w", err)
		}

		var ba actions.BrowserActions
		if err := json.Unmarshal(data, &ba); err != nil {
			// Try parsing as array directly
			if err := json.Unmarshal(data, &browserActions); err != nil {
				return fmt.Errorf("failed to parse actions file: %w", err)
			}
		} else {
			browserActions = ba.Actions
		}

		log.Info().
			Str("file", recordActionsFile).
			Int("actions", len(browserActions)).
			Msg("Loaded browser actions from file")
	}

	// If no actions but URL provided, create a simple navigate action
	if len(browserActions) == 0 && recordURL != "" {
		browserActions = []actions.Action{
			{
				Type: actions.ActionNavigate,
				URL:  recordURL,
			},
		}
	}

	// Print format info
	if videoFormat == browser.VideoFormatAuto {
		if browser.IsFFmpegAvailable() {
			log.Info().Msg("Using H.264/MP4 format (ffmpeg available)")
		} else {
			log.Info().Msg("Using MJPEG/AVI format (ffmpeg not available)")
		}
	}

	// Setup output directory
	outputDir, err := os.MkdirTemp("", "sukyan-record-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	if !recordKeepFrames {
		defer os.RemoveAll(outputDir)
	} else {
		log.Info().Str("dir", outputDir).Msg("Frames will be kept in directory")
	}

	// Ensure output directory exists for the video file
	if dir := filepath.Dir(recordOutput); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Launch browser
	log.Info().Msg("Launching browser...")
	launcher := browser.GetBrowserLauncher()
	controlURL, err := launcher.Launch()
	if err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	rodBrowser := rod.New().ControlURL(controlURL).MustConnect()
	defer rodBrowser.MustClose()

	page := rodBrowser.MustPage("")
	defer page.MustClose()

	// Debug: capture console output
	go page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) bool {
		for _, arg := range e.Args {
			log.Debug().Interface("value", arg.Value).Msg("Browser console")
		}
		return false
	})()

	// Set viewport if specified
	if recordWidth > 0 || recordHeight > 0 {
		width := recordWidth
		height := recordHeight
		if width == 0 {
			width = 1280
		}
		if height == 0 {
			height = 800
		}
		page.MustSetViewport(width, height, 1.0, false)
	}

	// Configure recording options
	screencastOpts := &browser.ScreencastOptions{
		Format:        browser.ScreencastFormatJPEG,
		Quality:       recordQuality,
		EveryNthFrame: 1,
		BufferSize:    200,
	}
	if recordWidth > 0 {
		screencastOpts.MaxWidth = recordWidth
	}
	if recordHeight > 0 {
		screencastOpts.MaxHeight = recordHeight
	}

	pocOpts := &browser.ProofOfConceptOptions{
		OutputDir:         outputDir,
		EncodeVideo:       true,
		CleanupFrames:     !recordKeepFrames,
		VideoFilename:     filepath.Base(recordOutput),
		ScreencastOptions: screencastOpts,
		VideoEncoderOptions: &browser.VideoEncoderOptions{
			Format:    videoFormat,
			Framerate: recordFPS,
		},
		PreActionDelay:     500 * time.Millisecond,
		PostActionDelay:    2 * time.Second,
		DialogDismissDelay: recordDialogDelay,
	}

	// Record
	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()

	log.Info().
		Int("actions", len(browserActions)).
		Str("output", recordOutput).
		Msg("Starting recording...")

	result, err := browser.RecordProofOfConcept(ctx, page, browserActions, pocOpts)
	if err != nil && result == nil {
		return fmt.Errorf("recording failed: %w", err)
	}

	// Move video to final location if it was created in temp dir
	if result.VideoPath != "" {
		// Determine final output path using the actual extension from encoding
		actualExt := filepath.Ext(result.VideoPath)
		requestedExt := filepath.Ext(recordOutput)
		finalOutput := recordOutput
		if actualExt != requestedExt {
			// Extension was adjusted during encoding (e.g., .avi -> .mp4)
			finalOutput = recordOutput[:len(recordOutput)-len(requestedExt)] + actualExt
		}

		// Copy to final destination
		data, err := os.ReadFile(result.VideoPath)
		if err != nil {
			return fmt.Errorf("failed to read generated video: %w", err)
		}
		if err := os.WriteFile(finalOutput, data, 0644); err != nil {
			return fmt.Errorf("failed to write video to %s: %w", finalOutput, err)
		}
		result.VideoPath = finalOutput
	}

	// Print results
	fmt.Println()
	fmt.Println("Recording complete!")
	fmt.Printf("  Frames captured: %d\n", result.FrameCount)
	fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))
	if result.VideoPath != "" {
		info, _ := os.Stat(result.VideoPath)
		if info != nil {
			fmt.Printf("  Video: %s (%s)\n", result.VideoPath, formatBytes(info.Size()))
		} else {
			fmt.Printf("  Video: %s\n", result.VideoPath)
		}
	}
	if recordKeepFrames {
		fmt.Printf("  Frames directory: %s\n", outputDir)
	}

	if result.Error != nil {
		log.Warn().Err(result.Error).Msg("Recording completed with errors")
	}

	if !result.ActionsResult.Succeded {
		log.Warn().Msg("Some browser actions may have failed")
	}

	return nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
