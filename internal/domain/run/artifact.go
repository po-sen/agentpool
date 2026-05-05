package run

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

const (
	// MaxArtifactCount is the maximum number of work files preserved for one run.
	MaxArtifactCount = 100
	// MaxArtifactSizeBytes is the maximum size preserved for one artifact.
	MaxArtifactSizeBytes int64 = 1 << 20
	// MaxTotalArtifactSizeBytes is the maximum total artifact content preserved for one run.
	MaxTotalArtifactSizeBytes int64 = 10 << 20
)

// Artifact is a bounded file captured from /workspace/work for POC run output access.
type Artifact struct {
	Path      string
	MediaType string
	Content   []byte
	SizeBytes int64
}

// Clone returns a detached copy of the artifact.
func (a Artifact) Clone() Artifact {
	clone := a
	if a.Content != nil {
		clone.Content = append([]byte(nil), a.Content...)
	}

	return clone
}

// ValidateArtifactPath checks an artifact path exposed through public API routes.
func ValidateArtifactPath(artifactPath string) error {
	if strings.TrimSpace(artifactPath) == "" ||
		artifactPath != strings.TrimSpace(artifactPath) ||
		path.IsAbs(artifactPath) ||
		filepath.IsAbs(artifactPath) ||
		filepath.VolumeName(artifactPath) != "" ||
		strings.Contains(artifactPath, "\\") ||
		strings.Contains(artifactPath, ":") ||
		path.Clean(artifactPath) != artifactPath {
		return errors.New("unsafe artifact path")
	}

	components := strings.Split(artifactPath, "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return errors.New("unsafe artifact path")
		}
	}

	return nil
}

func copyArtifacts(artifacts []Artifact) []Artifact {
	if len(artifacts) == 0 {
		return nil
	}

	copied := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		copied = append(copied, artifact.Clone())
	}

	return copied
}
