package reports

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Version struct {
	N          int     `json:"n"`
	Parent     *int    `json:"parent"`
	Frozen     bool    `json:"frozen"`
	ChangeNote *string `json:"change_note"`
	CreatedAt  *string `json:"created_at"`
}

type Meta struct {
	Tip           int       `json:"tip"`
	ActiveVersion int       `json:"active_version"`
	Etag          string    `json:"etag"`
	Versions      []Version `json:"versions"`
}

type Project struct {
	Root string
}

func (p Project) ReportsDir() string  { return filepath.Join(p.Root, "reports") }
func (p Project) TemplateDir() string { return filepath.Join(p.ReportsDir(), "template") }
func (p Project) MetaPath() string    { return filepath.Join(p.ReportsDir(), "meta.json") }
func (p Project) RegistryPath() string {
	return filepath.Join(p.ReportsDir(), "registry.ts")
}
func (p Project) VersionDir(n int) string {
	return filepath.Join(p.ReportsDir(), fmt.Sprintf("v%d", n))
}

// Open returns a Project rooted at dir. dir must contain a reports/ directory
// or a package.json (so init can run inside it).
func Open(dir string) (Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Project{}, err
	}
	return Project{Root: abs}, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadMeta loads meta.json. If missing, it initializes a v0 baseline by
// cloning reports/template -> reports/v0 (matching the Next.js behavior).
func (p Project) ReadMeta() (*Meta, error) {
	raw, err := os.ReadFile(p.MetaPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return p.initBaseline()
		}
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("meta.json: %w", err)
	}
	if m.Versions == nil {
		m.Versions = []Version{}
	}
	return &m, nil
}

func (p Project) initBaseline() (*Meta, error) {
	v0 := p.VersionDir(0)
	if !exists(filepath.Join(v0, "report.tsx")) {
		if !exists(p.TemplateDir()) {
			return nil, fmt.Errorf("no reports/template/ at %s — is this a numeral-reporting project?", p.Root)
		}
		if err := copyTree(p.TemplateDir(), v0); err != nil {
			return nil, fmt.Errorf("clone template -> v0: %w", err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	m := &Meta{
		Tip:           0,
		ActiveVersion: 0,
		Versions: []Version{{
			N:         0,
			Frozen:    false,
			CreatedAt: &now,
		}},
	}
	if err := p.WriteMeta(m); err != nil {
		return nil, err
	}
	if err := p.RefreshRegistry(); err != nil {
		return nil, err
	}
	return m, nil
}

func (p Project) WriteMeta(m *Meta) error {
	if err := os.MkdirAll(p.ReportsDir(), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	return os.WriteFile(p.MetaPath(), buf, 0o644)
}

// NewVersion clones from `from` (default tip) into the next vN slot.
func (p Project) NewVersion(from *int, name string) (created int, parent int, err error) {
	m, err := p.ReadMeta()
	if err != nil {
		return 0, 0, err
	}
	known := versionNumbers(m.Versions)
	next := -1
	for _, n := range known {
		if n > next {
			next = n
		}
	}
	next++

	src := m.Tip
	if from != nil {
		src = *from
	}
	if !containsInt(known, src) {
		return 0, 0, fmt.Errorf("unknown source version v%d", src)
	}

	srcDir := p.VersionDir(src)
	dstDir := p.VersionDir(next)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return 0, 0, err
	}
	if err := copyTree(srcDir, dstDir); err != nil {
		return 0, 0, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	parentCopy := src
	var note *string
	if s := strings.TrimSpace(name); s != "" {
		note = &s
	}
	m.Versions = append(m.Versions, Version{
		N:          next,
		Parent:     &parentCopy,
		Frozen:     false,
		ChangeNote: note,
		CreatedAt:  &now,
	})
	m.Tip = next
	m.ActiveVersion = next
	if err := p.WriteMeta(m); err != nil {
		return 0, 0, err
	}
	if err := p.RefreshRegistry(); err != nil {
		return 0, 0, err
	}
	return next, src, nil
}

func (p Project) Freeze(version int) error {
	m, err := p.ReadMeta()
	if err != nil {
		return err
	}
	for i := range m.Versions {
		if m.Versions[i].N == version {
			if m.Versions[i].Frozen {
				return nil
			}
			m.Versions[i].Frozen = true
			return p.WriteMeta(m)
		}
	}
	return fmt.Errorf("v%d not found", version)
}

func (p Project) Delete(version int) error {
	if version == 0 {
		return errors.New("v0 is the immutable baseline")
	}
	m, err := p.ReadMeta()
	if err != nil {
		return err
	}
	idx := -1
	for i := range m.Versions {
		if m.Versions[i].N == version {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("v%d not found", version)
	}
	if m.Versions[idx].Frozen {
		return fmt.Errorf("v%d is frozen", version)
	}
	if err := os.RemoveAll(p.VersionDir(version)); err != nil {
		return err
	}
	m.Versions = append(m.Versions[:idx], m.Versions[idx+1:]...)
	if m.Tip == version {
		max := 0
		for _, v := range m.Versions {
			if v.N > max {
				max = v.N
			}
		}
		m.Tip = max
	}
	if m.ActiveVersion == version {
		m.ActiveVersion = m.Tip
	}
	if err := p.WriteMeta(m); err != nil {
		return err
	}
	return p.RefreshRegistry()
}

func (p Project) SetActive(version int) error {
	m, err := p.ReadMeta()
	if err != nil {
		return err
	}
	if !containsInt(versionNumbers(m.Versions), version) {
		return fmt.Errorf("v%d not found", version)
	}
	if m.ActiveVersion == version {
		return nil
	}
	m.ActiveVersion = version
	return p.WriteMeta(m)
}

// RefreshRegistry rewrites reports/registry.ts to match versions on disk.
func (p Project) RefreshRegistry() error {
	if err := os.MkdirAll(p.ReportsDir(), 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(p.ReportsDir())
	if err != nil {
		return err
	}
	var versions []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "v") {
			continue
		}
		if _, err := strconv.Atoi(name[1:]); err != nil {
			continue
		}
		if exists(filepath.Join(p.ReportsDir(), name, "report.tsx")) {
			versions = append(versions, name)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		a, _ := strconv.Atoi(versions[i][1:])
		b, _ := strconv.Atoi(versions[j][1:])
		return a < b
	})

	var b strings.Builder
	b.WriteString(`import type { ComponentType } from "react";` + "\n")
	b.WriteString(`import type { ReportVersionComponentProps } from "@/schemas/report";` + "\n\n")
	b.WriteString("export type ReportModule = { default: ComponentType<ReportVersionComponentProps> };\n\n")
	b.WriteString("export const reportRegistry = {\n")
	for _, v := range versions {
		fmt.Fprintf(&b, "  %s: () => import(\"./%s/report\"),\n", v, v)
	}
	b.WriteString("} satisfies Record<string, () => Promise<ReportModule>>;\n")
	return os.WriteFile(p.RegistryPath(), []byte(b.String()), 0o644)
}

func versionNumbers(vs []Version) []int {
	out := make([]int, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.N)
	}
	return out
}

func containsInt(xs []int, n int) bool {
	for _, x := range xs {
		if x == n {
			return true
		}
	}
	return false
}

// copyTree recursively copies src to dst. dst is created if missing.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
