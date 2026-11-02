package site

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Stable buckets keep one changed title from repacking all the other cards.
// A pack holds about 60 JPEGs in the production corpus. Its content hash,
// offset and length form the public image URL, so old URLs keep working.
const socialPackCount = 128

func socialPackPath(hash string, offset, length int, tag string) string {
	return fmt.Sprintf("social/%s/%d/%d/%s.jpg", hash, offset, length, tag)
}

// writeSocialPacks replaces individual JPEGs with content-addressed packs.
// It leaves a pack in Pages for fallback delivery and optionally writes a copy
// for R2 upload. The Worker gets the exact byte range from each image URL.
func (b *builder) writeSocialPacks() error {
	dir := filepath.Join(b.opt.Out, "social")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var buckets [socialPackCount][]string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		tag, _, ok := strings.Cut(e.Name(), "-")
		if !ok {
			return fmt.Errorf("site: social image %q has no tag", e.Name())
		}
		i := shardOf(tag, socialPackCount)
		buckets[i] = append(buckets[i], e.Name())
	}
	if b.opt.SocialPacks != "" {
		if err := os.MkdirAll(b.opt.SocialPacks, 0o750); err != nil {
			return err
		}
	}
	for _, names := range buckets {
		if len(names) == 0 {
			continue
		}
		slices.Sort(names)
		var pack bytes.Buffer
		urls := make(map[string][2]int, len(names))
		for _, name := range names {
			p := filepath.Join(dir, name)
			data, err := os.ReadFile(p) // #nosec G304 -- name is from ReadDir in the generated site.
			if err != nil {
				return err
			}
			urls[name] = [2]int{pack.Len(), len(data)}
			if _, err := pack.Write(data); err != nil {
				return err
			}
			if err := os.Remove(p); err != nil {
				return err
			}
		}
		h := sha256.Sum256(pack.Bytes())
		hash := hex.EncodeToString(h[:12])
		if err := b.write("social/packs/"+hash+".pack", pack.Bytes()); err != nil {
			return err
		}
		if b.opt.SocialPacks != "" {
			if err := os.WriteFile(filepath.Join(b.opt.SocialPacks, hash+".pack"), pack.Bytes(), 0o644); err != nil { // #nosec G306 -- packs contain public preview images.
				return err
			}
		}
		for name, span := range urls {
			tag, _, _ := strings.Cut(name, "-")
			url := b.abs(socialPackPath(hash, span[0], span[1], tag))
			if tag == "site" {
				b.socialImage = url
			} else {
				b.socialURLs[tag] = url
			}
		}
	}
	return nil
}
