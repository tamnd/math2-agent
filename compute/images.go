package compute

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"go.yaml.in/yaml/v3"
)

// ImagesFile is the path of the image manifest from the tree root.
const ImagesFile = "manifests/images.yaml"

// Arches are the architectures of the runners, as GOARCH names them (spec 08.6).
var Arches = []string{"amd64", "arm64"}

// Image is one image of the manifest.
type Image struct {
	Name     string            `yaml:"name"`
	Contents string            `yaml:"contents"`
	Digests  map[string]string `yaml:",inline"` // sha256:HEX by architecture
}

// Images is the image manifest, manifests/images.yaml.
type Images struct {
	Images []Image `yaml:"images"`
}

var (
	digestRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	nameRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

// ReadImages reads manifests/images.yaml of the tree at root. A tree without the file gives an error that wraps fs.ErrNotExist.
func ReadImages(root string) (*Images, error) {
	data, err := os.ReadFile(filepath.Join(root, ImagesFile)) // #nosec G304 -- a fixed path in the tree that the caller names.
	if err != nil {
		return nil, err
	}
	return ParseImages(data)
}

// ParseImages parses an image manifest and checks it: each image has a name that no other image has, and a digest for each architecture is sha256:HEX.
func ParseImages(data []byte) (*Images, error) {
	var im Images
	if err := yaml.Unmarshal(data, &im); err != nil {
		return nil, fmt.Errorf("compute: %s: %w", ImagesFile, err)
	}
	seen := map[string]bool{}
	for _, img := range im.Images {
		if !nameRe.MatchString(img.Name) {
			return nil, fmt.Errorf("compute: %s: %q is not a valid image name", ImagesFile, img.Name)
		}
		if seen[img.Name] {
			return nil, fmt.Errorf("compute: %s: the image %s is there two times", ImagesFile, img.Name)
		}
		seen[img.Name] = true
		for arch, d := range img.Digests {
			if !slices.Contains(Arches, arch) {
				return nil, fmt.Errorf("compute: %s: %s has the key %q, and the architectures are amd64 and arm64", ImagesFile, img.Name, arch)
			}
			if !digestRe.MatchString(d) {
				return nil, fmt.Errorf("compute: %s: the %s digest of %s is %q, not sha256:HEX", ImagesFile, arch, img.Name, d)
			}
		}
	}
	return &im, nil
}

// Find returns the image that the reference name@sha256:HEX names, and the architecture of the digest. It returns false when no image of the manifest has that name and that digest.
func (im *Images) Find(ref string) (img *Image, arch string, ok bool) {
	name, digest, ok := SplitRef(ref)
	if !ok {
		return nil, "", false
	}
	for i := range im.Images {
		if im.Images[i].Name != name {
			continue
		}
		for _, a := range Arches {
			if im.Images[i].Digests[a] == digest {
				return &im.Images[i], a, true
			}
		}
	}
	return nil, "", false
}

// Named returns the image of the manifest with the name, or nil.
func (im *Images) Named(name string) *Image {
	for i := range im.Images {
		if im.Images[i].Name == name {
			return &im.Images[i]
		}
	}
	return nil
}
