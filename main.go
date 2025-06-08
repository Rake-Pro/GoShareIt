package main

import (
	"fmt"
	"github.com/getlantern/systray"
	"github.com/rake8288/goshareit/capture"
	"github.com/rake8288/goshareit/config"
	"github.com/rake8288/goshareit/logger"
	"github.com/rake8288/goshareit/uploader"
	"github.com/rake8288/goshareit/utilities"
	"os"
	"time"
)

var (
	recording        = false
	recordFullItem   *systray.MenuItem
	recordSelectItem *systray.MenuItem
)

func onReady() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(fmt.Sprintf("Panic in onReady: %v", r))
		}
	}()

	logger.Info("System tray ready")

	systray.SetTitle("GoShareIt")
	systray.SetTooltip("GoShareIt - macOS screenshot & screen recording uploader")

	fullShotItem := systray.AddMenuItem("Capture Screenshot Full", "Take full screenshot")
	selectShotItem := systray.AddMenuItem("Capture Screenshot Region", "Take selective screenshot")
	systray.AddSeparator()
	recordFullItem = systray.AddMenuItem("Start Full Screen Recording", "Start full screen recording")
	recordSelectItem = systray.AddMenuItem("Start Screen Region Recording", "Start selective screen recording")
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("Quit", "Exit GoShareIt")

	go func() {
		for {
			select {
			case <-fullShotItem.ClickedCh:
				go handleScreenshot(false)
			case <-selectShotItem.ClickedCh:
				go handleScreenshot(true)
			case <-recordFullItem.ClickedCh:
				go toggleRecording(false)
			case <-recordSelectItem.ClickedCh:
				go toggleRecording(true)
			case <-quitItem.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func toggleRecording(selective bool) {
	if recording {
		capture.StopScreenRecording()
		recordFullItem.SetTitle("Start Full Recording")
		recordSelectItem.SetTitle("Start Region Recording")
		recording = false
	} else {
		recording = true
		if selective {
			recordSelectItem.SetTitle("Stop Region Recording")
		} else {
			recordFullItem.SetTitle("Stop Full Recording")
		}
		capture.StartScreenRecording(selective)
	}
}

func handleScreenshot(selective bool) {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("/tmp/screenshot_%s.png", timestamp)

	logger.Info("Handling screenshot: " + filename)
	err := capture.TakeScreenshot(filename, selective)
	if err != nil {
		logger.Error("Screenshot failed: " + err.Error())
		utilities.Notify("Screenshot failed", err.Error())
		return
	}
	info, statErr := os.Stat(filename)
	if statErr != nil || info.Size() == 0 {
		logger.Warn("Screenshot canceled or empty file: " + filename)
		//utilities.Notify("Screenshot cancelled", "No screenshot was saved.") // No need to notify user unless debugging
		return
	}

	logger.Info("Screenshot saved: " + filename)

	url, err := uploader.UploadToNextcloud(filename)
	if err != nil {
		logger.Error("Upload failed: " + err.Error())
		utilities.Notify("Upload failed", err.Error())
		return
	}

	utilities.CopyToClipboard(url)
	logger.Info("Upload complete, URL copied to clipboard: " + url)
	utilities.Notify("Uploaded!", "URL copied to clipboard"+url)
}

func main() {
	logger.InitLogger()
	logger.Debug("Starting GoShareIt")

	config.LoadConfig()

	onExit := func() {
		logger.Info("Exiting...")
		logger.CloseLogger()
	}

	systray.Run(onReady, onExit)
}
