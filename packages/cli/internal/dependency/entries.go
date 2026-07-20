package dependency

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	projectarchive "github.com/billstark001/latexmk/packages/cli/internal/archive"
)

const (
	maxReportedEntryCandidates = 256
	maxReportedEntryWarnings   = 32
)

// EntryCandidate is a policy-approved TeX file that may be a root document.
// Reason describes deterministic evidence and never contains file contents.
type EntryCandidate struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Reason string `json:"reason"`
}

// EntryDiscovery is a bounded, deterministic entry-file discovery result.
// Selected is set only when there is one unambiguous candidate.
type EntryDiscovery struct {
	Status         string           `json:"status"`
	Selected       string           `json:"selected,omitempty"`
	Unambiguous    bool             `json:"unambiguous"`
	TexFileCount   int              `json:"texFileCount"`
	CandidateCount int              `json:"candidateCount"`
	Candidates     []EntryCandidate `json:"candidates"`
	Truncated      bool             `json:"truncated,omitempty"`
	Warnings       []string         `json:"warnings,omitempty"`
}

// DiscoverEntries identifies root-document candidates from a policy-filtered
// manifest. It does not build or modify a dependency set.
func DiscoverEntries(files []projectarchive.File) (EntryDiscovery, error) {
	texFiles := make([]projectarchive.File, 0)
	for _, file := range files {
		if strings.EqualFold(path.Ext(file.Path), ".tex") {
			texFiles = append(texFiles, file)
		}
	}
	sort.Slice(texFiles, func(i, j int) bool { return texFiles[i].Path < texFiles[j].Path })

	result := EntryDiscovery{Status: "not_found", TexFileCount: len(texFiles), Candidates: []EntryCandidate{}}
	if len(texFiles) == 0 {
		return result, nil
	}

	strong := make([]EntryCandidate, 0)
	unknown := make([]EntryCandidate, 0)
	warnings := entryWarningCollector{}
	for _, file := range texFiles {
		content, inspected, err := readEntryCandidate(file)
		if err != nil {
			return EntryDiscovery{}, fmt.Errorf("read entry candidate %s: %w", file.Path, err)
		}
		if !inspected {
			unknown = append(unknown, EntryCandidate{Path: file.Path, Size: file.Size, Reason: "not-inspected-too-large"})
			warnings.add(fmt.Sprintf("%s exceeds the %d-byte entry scanner limit", file.Path, maxParsedFileSize))
			continue
		}
		if containsDocumentClass(sanitize(content)) {
			strong = append(strong, EntryCandidate{Path: file.Path, Size: file.Size, Reason: "documentclass"})
		}
	}

	var candidates []EntryCandidate
	switch {
	case len(strong) > 0:
		candidates = append(candidates, strong...)
		candidates = append(candidates, unknown...)
	case len(texFiles) == 1 && len(unknown) == 0:
		candidates = []EntryCandidate{{Path: texFiles[0].Path, Size: texFiles[0].Size, Reason: "only-tex-file"}}
	default:
		for _, file := range texFiles {
			reason := "tex-file"
			if file.Size > maxParsedFileSize {
				reason = "not-inspected-too-large"
			}
			candidates = append(candidates, EntryCandidate{Path: file.Path, Size: file.Size, Reason: reason})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
	result.CandidateCount = len(candidates)
	if len(candidates) == 1 && candidates[0].Reason != "not-inspected-too-large" {
		result.Status = "selected"
		result.Selected = candidates[0].Path
		result.Unambiguous = true
	} else {
		result.Status = "ambiguous"
	}
	if len(candidates) > maxReportedEntryCandidates {
		result.Candidates = append(result.Candidates, candidates[:maxReportedEntryCandidates]...)
		result.Truncated = true
		warnings.add(fmt.Sprintf("entry candidate output is limited to %d paths", maxReportedEntryCandidates))
	} else {
		result.Candidates = append(result.Candidates, candidates...)
	}
	result.Warnings = warnings.result()
	return result, nil
}

type entryWarningCollector struct {
	messages []string
	omitted  int
}

func (c *entryWarningCollector) add(message string) {
	if len(c.messages) < maxReportedEntryWarnings-1 {
		c.messages = append(c.messages, message)
		return
	}
	c.omitted++
}

func (c *entryWarningCollector) result() []string {
	result := append([]string(nil), c.messages...)
	if c.omitted > 0 {
		result = append(result, fmt.Sprintf("%d additional entry discovery warnings omitted", c.omitted))
	}
	return result
}

func readEntryCandidate(file projectarchive.File) (string, bool, error) {
	info, err := os.Lstat(file.Source)
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("source is not a regular file")
	}
	if info.Size() != file.Size {
		return "", false, fmt.Errorf("source size changed after the upload manifest was built")
	}
	if info.Size() > maxParsedFileSize {
		return "", false, nil
	}

	source, err := os.Open(file.Source)
	if err != nil {
		return "", false, err
	}
	defer source.Close()
	openedInfo, err := source.Stat()
	if err != nil {
		return "", false, err
	}
	if !openedInfo.Mode().IsRegular() || openedInfo.Size() != file.Size {
		return "", false, fmt.Errorf("source changed after the upload manifest was built")
	}
	content, err := io.ReadAll(io.LimitReader(source, maxParsedFileSize+1))
	if err != nil {
		return "", false, err
	}
	if int64(len(content)) != file.Size || len(content) > maxParsedFileSize {
		return "", false, fmt.Errorf("source changed after the upload manifest was built")
	}
	if file.SHA256 != "" {
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != strings.ToLower(file.SHA256) {
			return "", false, fmt.Errorf("source content changed after the upload manifest was built")
		}
	}
	return string(content), true, nil
}

func containsDocumentClass(text string) bool {
	for _, call := range scanInvocations(text) {
		if call.name == "documentclass" && !call.malformed && len(call.args) == 1 {
			return true
		}
	}
	return false
}
