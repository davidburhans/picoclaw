package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillsInfoValidate(t *testing.T) {
	testcases := []struct {
		name        string
		skillName   string
		description string
		wantErr     bool
		errContains []string
	}{
		{
			name:        "valid-skill",
			skillName:   "valid-skill",
			description: "a valid skill description",
			wantErr:     false,
		},
		{
			name:        "empty-name",
			skillName:   "",
			description: "description without name",
			wantErr:     true,
			errContains: []string{"name is required"},
		},
		{
			name:        "empty-description",
			skillName:   "skill-without-description",
			description: "",
			wantErr:     true,
			errContains: []string{"description is required"},
		},
		{
			name:        "empty-both",
			skillName:   "",
			description: "",
			wantErr:     true,
			errContains: []string{"name is required", "description is required"},
		},
		{
			name:        "name-with-spaces",
			skillName:   "skill with spaces",
			description: "invalid name with spaces",
			wantErr:     true,
			errContains: []string{"name must be alphanumeric with hyphens"},
		},
		{
			name:        "name-with-underscore",
			skillName:   "skill_underscore",
			description: "invalid name with underscore",
			wantErr:     true,
			errContains: []string{"name must be alphanumeric with hyphens"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			info := SkillInfo{
				Name:        tc.skillName,
				Description: tc.description,
			}
			err := info.validate()
			if tc.wantErr {
				assert.Error(t, err)
				for _, msg := range tc.errContains {
					assert.ErrorContains(t, err, msg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtractFrontmatter(t *testing.T) {
	sl := &SkillsLoader{}

	testcases := []struct {
		name           string
		content        string
		expectedName   string
		expectedDesc   string
		lineEndingType string
	}{
		{
			name:           "unix-line-endings",
			lineEndingType: "Unix (\\n)",
			content:        "---\nname: test-skill\ndescription: A test skill\n---\n\n# Skill Content",
			expectedName:   "test-skill",
			expectedDesc:   "A test skill",
		},
		{
			name:           "windows-line-endings",
			lineEndingType: "Windows (\\r\\n)",
			content:        "---\r\nname: test-skill\r\ndescription: A test skill\r\n---\r\n\r\n# Skill Content",
			expectedName:   "test-skill",
			expectedDesc:   "A test skill",
		},
		{
			name:           "classic-mac-line-endings",
			lineEndingType: "Classic Mac (\\r)",
			content:        "---\rname: test-skill\rdescription: A test skill\r---\r\r# Skill Content",
			expectedName:   "test-skill",
			expectedDesc:   "A test skill",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			frontmatter := sl.extractFrontmatter(tc.content)
			assert.NotEmpty(t, frontmatter, "Frontmatter should be extracted for %s line endings", tc.lineEndingType)

			yamlMeta := sl.parseSimpleYAML(frontmatter)
			assert.Equal(t, tc.expectedName, yamlMeta["name"], "Name should be correctly parsed from frontmatter with %s line endings", tc.lineEndingType)
			assert.Equal(t, tc.expectedDesc, yamlMeta["description"], "Description should be correctly parsed from frontmatter with %s line endings", tc.lineEndingType)
		})
	}
}

func TestStripFrontmatter(t *testing.T) {
	sl := &SkillsLoader{}

	testcases := []struct {
		name            string
		content         string
		expectedContent string
		lineEndingType  string
	}{
		{
			name:            "unix-line-endings",
			lineEndingType:  "Unix (\\n)",
			content:         "---\nname: test-skill\ndescription: A test skill\n---\n\n# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "windows-line-endings",
			lineEndingType:  "Windows (\\r\\n)",
			content:         "---\r\nname: test-skill\r\ndescription: A test skill\r\n---\r\n\r\n# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "classic-mac-line-endings",
			lineEndingType:  "Classic Mac (\\r)",
			content:         "---\rname: test-skill\rdescription: A test skill\r---\r\r# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "unix-line-endings-without-trailing-newline",
			lineEndingType:  "Unix (\\n) without trailing newline",
			content:         "---\nname: test-skill\ndescription: A test skill\n---\n# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "windows-line-endings-without-trailing-newline",
			lineEndingType:  "Windows (\\r\\n) without trailing newline",
			content:         "---\r\nname: test-skill\r\ndescription: A test skill\r\n---\r\n# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "no-frontmatter",
			lineEndingType:  "No frontmatter",
			content:         "# Skill Content\n\nSome content here.",
			expectedContent: "# Skill Content\n\nSome content here.",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			result := sl.stripFrontmatter(tc.content)
			assert.Equal(t, tc.expectedContent, result, "Frontmatter should be stripped correctly for %s", tc.lineEndingType)
		})
	}
}

// makeSkill creates a minimal skill directory under base for testing.
func makeSkill(t *testing.T, base, name string, extraFiles map[string]string) {
	t.Helper()
	dir := filepath.Join(base, name)
	require.NoError(t, os.MkdirAll(dir, 0755))
	skillMD := "---\nname: " + name + "\ndescription: Test skill " + name + "\n---\n\n# " + name
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))
	for relPath, content := range extraFiles {
		full := filepath.Join(dir, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0644))
	}
}

func TestSkillsLoader_GetSkillDir(t *testing.T) {
	base := t.TempDir()
	wsSkills := filepath.Join(base, "workspace", "skills")
	globalSkills := filepath.Join(base, "global", "skills")

	makeSkill(t, wsSkills, "my-skill", nil)
	makeSkill(t, globalSkills, "global-skill", nil)

	sl := NewSkillsLoader(filepath.Join(base, "workspace"), globalSkills, "")

	t.Run("workspace skill found", func(t *testing.T) {
		dir, ok := sl.GetSkillDir("my-skill")
		require.True(t, ok)
		assert.Equal(t, filepath.Join(wsSkills, "my-skill"), dir)
	})

	t.Run("global skill found", func(t *testing.T) {
		dir, ok := sl.GetSkillDir("global-skill")
		require.True(t, ok)
		assert.Equal(t, filepath.Join(globalSkills, "global-skill"), dir)
	})

	t.Run("missing skill returns false", func(t *testing.T) {
		_, ok := sl.GetSkillDir("nonexistent")
		assert.False(t, ok)
	})

	t.Run("workspace overrides global", func(t *testing.T) {
		makeSkill(t, wsSkills, "shared-skill", nil)
		makeSkill(t, globalSkills, "shared-skill", nil)
		dir, ok := sl.GetSkillDir("shared-skill")
		require.True(t, ok)
		assert.Equal(t, filepath.Join(wsSkills, "shared-skill"), dir)
	})
}

func TestSkillsLoader_ListSkillFiles(t *testing.T) {
	base := t.TempDir()
	wsSkills := filepath.Join(base, "workspace", "skills")
	makeSkill(t, wsSkills, "rich-skill", map[string]string{
		"scripts/helper.sh":      "#!/bin/sh\necho hello",
		"references/api-docs.md": "# API",
		"assets/logo.png":        "PNG",
	})

	sl := NewSkillsLoader(filepath.Join(base, "workspace"), "", "")
	dir, ok := sl.GetSkillDir("rich-skill")
	require.True(t, ok)

	files, err := sl.ListSkillFiles(dir)
	require.NoError(t, err)

	// SKILL.md must be excluded
	for _, f := range files {
		assert.NotEqual(t, "SKILL.md", f, "SKILL.md should be excluded from listing")
	}

	// Normalize to forward slashes for cross-platform comparison
	var slashFiles []string
	for _, f := range files {
		slashFiles = append(slashFiles, filepath.ToSlash(f))
	}
	assert.Contains(t, slashFiles, "assets/logo.png")
	assert.Contains(t, slashFiles, "references/api-docs.md")
	assert.Contains(t, slashFiles, "scripts/helper.sh")
	assert.Equal(t, []string{"assets/logo.png", "references/api-docs.md", "scripts/helper.sh"}, slashFiles)
}

func TestSkillsLoader_ListSkillFiles_Empty(t *testing.T) {
	base := t.TempDir()
	wsSkills := filepath.Join(base, "workspace", "skills")
	makeSkill(t, wsSkills, "bare-skill", nil)

	sl := NewSkillsLoader(filepath.Join(base, "workspace"), "", "")
	dir, ok := sl.GetSkillDir("bare-skill")
	require.True(t, ok)

	files, err := sl.ListSkillFiles(dir)
	require.NoError(t, err)
	assert.Empty(t, files, "no files besides SKILL.md should yield empty listing")
}
