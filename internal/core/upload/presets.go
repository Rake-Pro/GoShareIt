package upload

// Preset is a starter configuration for a public image/GIF/file host, plus
// the notes the settings UI shows next to it. The CustomConfig is complete
// except for credentials: {secret} placeholders are filled from the Secret
// field at runtime, so a preset never carries a key.
type Preset struct {
	CustomConfig
	Label          string // display name
	Kinds          string // what the host accepts, e.g. "images, GIFs, video"
	Secret         string // what to put in the Secret field; "" when the host needs none
	SecretOptional bool   // the preset works without a secret (anonymous)
	Help           string // where to get the key, limits, retention
}

// CustomPresets returns the built-in hosts, keyed by a short id. They are
// data for the settings UI to prefill and edit, not ready-to-use credentials.
// Endpoints reflect each host's public API at the time of writing; a host that
// changes its API only needs the preset updated, never code.
func CustomPresets() map[string]Preset {
	return map[string]Preset{
		"imgur": {
			Label:  "Imgur",
			Kinds:  "images, GIFs, video",
			Secret: "Imgur application Client ID",
			Help: "Register an application at api.imgur.com/oauth2/addclient (choose anonymous usage) and paste the Client ID as the secret. " +
				"Anonymous uploads are not tied to an account; keep the deletion link from history to remove them. " +
				"For MP4 recordings change the file field to \"video\".",
			CustomConfig: CustomConfig{
				Method:                    "POST",
				URL:                       "https://api.imgur.com/3/image",
				Headers:                   map[string]string{"Authorization": "Client-ID {secret}"},
				Body:                      "multipart",
				FileField:                 "image",
				ResponseURLPath:           "data.link",
				ResponseDeleteURLTemplate: "https://imgur.com/delete/{json:data.deletehash}",
			},
		},
		"imgbb": {
			Label:  "ImgBB",
			Kinds:  "images, GIFs (32 MB)",
			Secret: "ImgBB API key",
			Help:   "Free key at api.imgbb.com after signing in. Uploads are kept indefinitely unless you add an expiration field; the deletion link is stored in history.",
			CustomConfig: CustomConfig{
				Method:                "POST",
				URL:                   "https://api.imgbb.com/1/upload?key={secret}",
				Body:                  "multipart",
				FileField:             "image",
				ResponseURLPath:       "data.url_viewer",
				ResponseDirectURLPath: "data.url",
				ResponseDeleteURLPath: "data.delete_url",
			},
		},
		"imgchest": {
			Label:  "Imgchest",
			Kinds:  "images, GIFs",
			Secret: "Imgchest API key",
			Help:   "Create an account at imgchest.com, then generate a key under Settings > API. Uploads go to your account as posts.",
			CustomConfig: CustomConfig{
				Method:                "POST",
				URL:                   "https://api.imgchest.com/v1/post",
				Headers:               map[string]string{"Authorization": "Bearer {secret}"},
				Body:                  "multipart",
				FileField:             "images[]",
				ResponseURLTemplate:   "https://imgchest.com/p/{json:data.id}",
				ResponseDirectURLPath: "data.images.0.link",
			},
		},
		"lensdump": {
			Label:  "Lensdump",
			Kinds:  "images, GIFs",
			Secret: "Lensdump API key",
			Help:   "Sign in at lensdump.com and copy the key from Settings > API. Lensdump runs Chevereto, so the generic Chevereto preset works for any other Chevereto host.",
			CustomConfig: CustomConfig{
				Method:                "POST",
				URL:                   "https://lensdump.com/api/1/upload?key={secret}",
				Body:                  "multipart",
				FileField:             "source",
				ResponseURLPath:       "image.url_viewer",
				ResponseDirectURLPath: "image.url",
				ResponseDeleteURLPath: "image.delete_url",
			},
		},
		"chevereto": {
			Label:  "Chevereto (any host)",
			Kinds:  "images, GIFs",
			Secret: "API v1 key from the Chevereto site",
			Help:   "Replace your.chevereto.host in the URL with the site you use (many public image hosts run Chevereto). The key is under your profile > Settings > API on that site.",
			CustomConfig: CustomConfig{
				Method:                "POST",
				URL:                   "https://your.chevereto.host/api/1/upload?key={secret}",
				Body:                  "multipart",
				FileField:             "source",
				ResponseURLPath:       "image.url_viewer",
				ResponseDirectURLPath: "image.url",
				ResponseDeleteURLPath: "image.delete_url",
			},
		},
		"catbox": {
			Label:          "Catbox",
			Kinds:          "any file (200 MB), kept indefinitely",
			Secret:         "Catbox userhash (optional)",
			SecretOptional: true,
			Help:           "Works anonymously. To file uploads under your Catbox account, add an extra field userhash={secret} and paste the userhash from catbox.moe/user/manage.php as the secret.",
			CustomConfig: CustomConfig{
				Method:      "POST",
				URL:         "https://catbox.moe/user/api.php",
				Body:        "multipart",
				FileField:   "fileToUpload",
				ExtraFields: map[string]string{"reqtype": "fileupload"},
			},
		},
		"litterbox": {
			Label:          "Litterbox (temporary)",
			Kinds:          "any file (1 GB), deleted after 24 h",
			SecretOptional: true,
			Help:           "Catbox's temporary service, no account. Change the time field to 1h, 12h, 24h or 72h.",
			CustomConfig: CustomConfig{
				Method:      "POST",
				URL:         "https://litterbox.catbox.moe/resources/internals/api.php",
				Body:        "multipart",
				FileField:   "fileToUpload",
				ExtraFields: map[string]string{"reqtype": "fileupload", "time": "24h"},
			},
		},
		"uguu": {
			Label:          "Uguu (temporary)",
			Kinds:          "any file (128 MB), deleted after 3 h",
			SecretOptional: true,
			Help:           "No account. Files expire after three hours, which suits one-off sharing.",
			CustomConfig: CustomConfig{
				Method:          "POST",
				URL:             "https://uguu.se/upload",
				Body:            "multipart",
				FileField:       "files[]",
				ResponseURLPath: "files.0.url",
			},
		},
		"0x0": {
			Label:          "0x0.st",
			Kinds:          "any file (512 MB), retention 30 to 365 days by size",
			SecretOptional: true,
			Help:           "No account. Smaller files stay longer. Deletion is by token from the response header X-Token, not exposed here.",
			CustomConfig: CustomConfig{
				Method:    "POST",
				URL:       "https://0x0.st",
				Body:      "multipart",
				FileField: "file",
			},
		},
		"pixeldrain": {
			Label:  "Pixeldrain",
			Kinds:  "any file (20 GB)",
			Secret: "Pixeldrain API key",
			Help:   "Create a key at pixeldrain.com/user/api_keys. Uploads are tied to your account and its retention rules.",
			CustomConfig: CustomConfig{
				Method:                    "PUT",
				URL:                       "https://pixeldrain.com/api/file/{name}",
				Headers:                   map[string]string{"Authorization": "Basic {secret:basic}"},
				Body:                      "raw",
				ResponseURLTemplate:       "https://pixeldrain.com/u/{json:id}",
				ResponseDirectURLTemplate: "https://pixeldrain.com/api/file/{json:id}",
			},
		},
		"gofile": {
			Label:          "Gofile",
			Kinds:          "any file, guest uploads expire when unused",
			Secret:         "Gofile account token (optional)",
			SecretOptional: true,
			Help:           "Works anonymously as a guest. To upload into your account, add a header Authorization=Bearer {secret} with the token from gofile.io/myProfile.",
			CustomConfig: CustomConfig{
				Method:          "POST",
				URL:             "https://upload.gofile.io/uploadfile",
				Body:            "multipart",
				FileField:       "file",
				ResponseURLPath: "data.downloadPage",
			},
		},
		"giphy": {
			Label:  "GIPHY",
			Kinds:  "GIFs and short video (100 MB), converted to GIF",
			Secret: "GIPHY API key",
			Help: "Create an app at developers.giphy.com and use its API key. Uploads land in your GIPHY account (unlisted until GIPHY indexes them); " +
				"the direct link plays the GIF anywhere. Use a GIF or video capture mode with this preset.",
			CustomConfig: CustomConfig{
				Method:                    "POST",
				URL:                       "https://upload.giphy.com/v1/gifs",
				Body:                      "multipart",
				FileField:                 "file",
				ExtraFields:               map[string]string{"api_key": "{secret}"},
				ResponseURLTemplate:       "https://giphy.com/gifs/{json:data.id}",
				ResponseDirectURLTemplate: "https://media.giphy.com/media/{json:data.id}/giphy.gif",
			},
		},
	}
}
