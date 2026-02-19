package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestSkillsLoaderFrontmatter(t *testing.T) {
	loader := &SkillsLoader{}

	testcases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "lf-line-endings",
			content: "---\nname: test\ndescription: desc\n---\n\n# Header",
			expected: "name: test\ndescription: desc",
		},
		{
			name: "crlf-line-endings",
			content: "---\r\nname: test\r\ndescription: desc\r\n---\r\n\n# Header",
			expected: "name: test\ndescription: desc",
		},
		{
			name: "mixed-line-endings",
			content: "---\r\nname: test\ndescription: desc\r\n---\n\n# Header",
			expected: "name: test\ndescription: desc",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := loader.extractFrontmatter(tc.content)
			assert.Equal(t, tc.expected, metadata)
		})
	}
}
