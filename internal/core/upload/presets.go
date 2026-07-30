package upload

// CustomPresets returns starter CustomConfig values for popular ShareX-style
// endpoints, keyed by a short id. They are data for the settings UI to
// prefill and edit (e.g. imgur's client ID is a literal placeholder), not
// ready-to-use credentials.
func CustomPresets() map[string]CustomConfig {
	return map[string]CustomConfig{
		"imgur": {
			Method:          "POST",
			URL:             "https://api.imgur.com/3/image",
			Headers:         map[string]string{"Authorization": "Client-ID YOUR_CLIENT_ID"},
			Body:            "multipart",
			FileField:       "image",
			ResponseURLPath: "data.link",
		},
		"catbox": {
			Method:      "POST",
			URL:         "https://catbox.moe/user/api.php",
			Body:        "multipart",
			FileField:   "fileToUpload",
			ExtraFields: map[string]string{"reqtype": "fileupload"},
		},
		"0x0": {
			Method:    "POST",
			URL:       "https://0x0.st",
			Body:      "multipart",
			FileField: "file",
		},
	}
}
