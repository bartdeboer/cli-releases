package cliget

import "time"

const manifestSchema = "cli-releases.manifest/v1"

type Manifest struct {
	Schema        string          `json:"schema"`
	Tool          string          `json:"tool"`
	Version       string          `json:"version"`
	SourceCommit  string          `json:"sourceCommit"`
	BuildTime     string          `json:"buildTime"`
	GoVersion     string          `json:"goVersion"`
	Artifacts     []Artifact      `json:"artifacts"`
	Documentation []Documentation `json:"documentation,omitempty"`
}
type Artifact struct {
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	File       string `json:"file"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	Executable string `json:"executable"`
}
type result struct {
	APIVersion  string    `json:"apiVersion"`
	ObservedAt  time.Time `json:"observedAt"`
	Status      string    `json:"status"`
	Tool        string    `json:"tool"`
	Version     string    `json:"version"`
	Platform    string    `json:"platform"`
	Destination string    `json:"destination"`
	SHA256      string    `json:"sha256"`
	Size        int64     `json:"size"`
	Overwritten bool      `json:"overwritten"`
}
type releaseResponse struct {
	Tag        string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}
type releaseAsset struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Digest      string `json:"digest"`
	ContentType string `json:"content_type"`
}

type Documentation struct {
	File   string `json:"file"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
