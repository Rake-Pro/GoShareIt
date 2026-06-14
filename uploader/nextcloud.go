package uploader

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Rake-Pro/goshareit/config"
	"github.com/Rake-Pro/goshareit/logger"
)

type ShareResponse struct {
	XMLName xml.Name `xml:"ocs"`
	Data    struct {
		URL string `xml:"url"`
	} `xml:"data"`
	Meta struct {
		Status     string `xml:"status"`
		StatusCode int    `xml:"statuscode"`
		Message    string `xml:"message"`
	} `xml:"meta"`
}

func UploadToNextcloud(filePath string) (string, error) {
	cfg := config.AppConfig.Nextcloud
	logger.Info("Starting upload for file: " + filePath)

	if _, err := os.Stat(filePath); err != nil {
		logger.Error("File not found: " + filePath)
		return "", fmt.Errorf("file not found: %w", err)
	}

	fileName := filepath.Base(filePath)
	uploadURL := strings.TrimRight(cfg.BaseURL, "/") + "/" + fileName
	//logger.Debug("Upload URL: " + uploadURL)

	file, err := os.Open(filePath)
	if err != nil {
		logger.Error("Failed to open file: " + err.Error())
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		logger.Error("Failed to read file contents: " + err.Error())
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	req, err := http.NewRequest("PUT", uploadURL, bytes.NewReader(fileData))
	if err != nil {
		logger.Error("Failed to create upload request: " + err.Error())
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error("Upload request failed: " + err.Error())
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("Upload failed [%d]: %s", resp.StatusCode, string(body))
		logger.Error(errMsg)
		return "", fmt.Errorf(errMsg)
	}

	shareURL := cfg.BaseURL
	if idx := strings.Index(shareURL, "/remote.php"); idx != -1 {
		shareURL = shareURL[:idx]
	}
	shareURL = strings.TrimRight(shareURL, "/") + "/ocs/v2.php/apps/files_sharing/api/v1/shares"

	form := url.Values{}
	form.Set("path", "/"+fileName)
	form.Set("shareType", "3")
	form.Set("permissions", "1")

	shareReq, err := http.NewRequest("POST", shareURL, strings.NewReader(form.Encode()))
	if err != nil {
		logger.Error("Failed to create share request: " + err.Error())
		return "", fmt.Errorf("failed to create share request: %w", err)
	}
	shareReq.SetBasicAuth(cfg.Username, cfg.Password)
	shareReq.Header.Set("OCS-APIRequest", "true")
	shareReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	shareResp, err := http.DefaultClient.Do(shareReq)
	if err != nil {
		logger.Error("Failed to send share request: " + err.Error())
		return "", fmt.Errorf("failed to send share request: %w", err)
	}
	defer shareResp.Body.Close()

	body, err := io.ReadAll(shareResp.Body)
	if err != nil {
		logger.Error("Failed to read share response: " + err.Error())
		return "", fmt.Errorf("failed to read share response: %w", err)
	}

	//logger.Debug("Raw share response: " + string(body))

	var shareRes ShareResponse
	err = xml.Unmarshal(body, &shareRes)
	if err != nil {
		logger.Error("Failed to parse share response XML: " + err.Error())
		logger.Error("Share response body: " + string(body))
		return "", fmt.Errorf("failed to parse share response: %w", err)
	}

	if shareRes.Meta.StatusCode != 100 && shareRes.Meta.StatusCode != 200 {
		errMsg := fmt.Sprintf("Failed to share file: %s", shareRes.Meta.Message)
		logger.Error(errMsg)
		logger.Error("Share response body: " + string(body))
		return "", fmt.Errorf(errMsg)
	}

	publicURL := strings.TrimRight(shareRes.Data.URL, "/")

	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" {
		publicURL += "/preview"
		//logger.Info("Upload successful with preview URL: " + publicURL)
	} else {
		//logger.Info("Upload successful with direct URL: " + publicURL)
	}

	return publicURL, nil
}
