package capture

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/rake8288/goshareit/logger"
	"github.com/rake8288/goshareit/uploader"
	"github.com/rake8288/goshareit/utilities"
)

var ffmpegPath = filepath.Join("bin", "ffmpeg")
var recordingCmd *exec.Cmd
var recordingMutex sync.Mutex
var currentOutputPath string
var isSelectiveRecording bool

func StartScreenRecording(selective bool) {
	timestamp := time.Now().Format("20060102_150405")
	outputPath := fmt.Sprintf("/tmp/recording_%s.mp4", timestamp)
	logger.Info("Starting screen recording: " + outputPath)

	var cmd *exec.Cmd
	isSelectiveRecording = selective
	currentOutputPath = outputPath

	if selective {
		regionCapturePath := "/tmp/region_select.png"
		captureCmd := exec.Command("screencapture", "-i", "-x", regionCapturePath)
		err := captureCmd.Run()
		if err != nil {
			logger.Error("Region selection failed: " + err.Error())
			utilities.Notify("Region selection failed", err.Error())
			return
		}

		sipsCmd := exec.Command("sips", "-g", "pixelWidth", "-g", "pixelHeight", regionCapturePath)
		output, err := sipsCmd.CombinedOutput()
		if err != nil {
			logger.Error("Failed to read image dimensions: " + err.Error())
			utilities.Notify("Could not determine region size", err.Error())
			return
		}

		width := extractSipsValue(string(output), "pixelWidth")
		height := extractSipsValue(string(output), "pixelHeight")
		if width == "" || height == "" {
			logger.Error("Could not extract width/height from sips output")
			utilities.Notify("Recording failed", "Region size could not be determined")
			return
		}

		videoSize := fmt.Sprintf("%sx%s", width, height)
		logger.Debug("Recording region size: " + videoSize)

		cmd = exec.Command(ffmpegPath,
			"-video_size", videoSize,
			"-framerate", "30",
			"-f", "avfoundation",
			"-i", "1:none",
			"-an",
			outputPath,
		)
	} else {
		cmd = exec.Command(ffmpegPath,
			"-f", "avfoundation",
			"-framerate", "30",
			"-i", "1:none",
			"-an",
			outputPath,
		)
	}

	recordingMutex.Lock()
	recordingCmd = cmd
	recordingMutex.Unlock()

	go func() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			logger.Error("Recording failed: " + err.Error())
			utilities.Notify("Recording failed", err.Error())
			return
		}
		logger.Info("Recording completed: " + currentOutputPath)
	}()
}

func StopScreenRecording() {
	recordingMutex.Lock()
	defer recordingMutex.Unlock()

	if recordingCmd == nil || recordingCmd.Process == nil {
		logger.Warn("No active recording process to stop")
		return
	}

	err := recordingCmd.Process.Signal(os.Interrupt)
	if err != nil {
		logger.Error("Failed to stop recording: " + err.Error())
		utilities.Notify("Failed to stop recording", err.Error())
		return
	}

	time.Sleep(1 * time.Second)

	logger.Info("Stopped recording: " + currentOutputPath)

	url, err := uploader.UploadToNextcloud(currentOutputPath)
	if err != nil {
		logger.Error("Upload failed: " + err.Error())
		utilities.Notify("Upload failed", err.Error())
		return
	}

	utilities.CopyToClipboard(url)
	utilities.Notify("Uploaded!", "URL copied to clipboard")
	logger.Info("Recording uploaded and link copied: " + url)

	recordingCmd = nil
}

func extractSipsValue(output, key string) string {
	re := regexp.MustCompile(key + ": (\\d+)")
	matches := re.FindStringSubmatch(output)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}
