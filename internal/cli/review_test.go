package cli

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestParseNameStatusZ(t *testing.T) {
	tests := []struct {
		name             string
		data             string
		expectedAdded    []string
		expectedModified []string
		expectedDeleted  []string
	}{
		{
			name:             "simple added, modified, deleted",
			data:             "A\x00file1.go\x00M\x00file2.go\x00D\x00file3.go\x00",
			expectedAdded:    []string{"file1.go"},
			expectedModified: []string{"file2.go"},
			expectedDeleted:  []string{"file3.go"},
		},
		{
			name:             "empty input",
			data:             "",
			expectedAdded:    nil,
			expectedModified: nil,
			expectedDeleted:  nil,
		},
		{
			name:             "unrecognized status ignored",
			data:             "X\x00file1.go\x00A\x00file2.go\x00",
			expectedAdded:    []string{"file2.go"},
			expectedModified: nil,
			expectedDeleted:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			added, modified, deleted := parseNameStatusZ(tc.data)
			if !reflect.DeepEqual(added, tc.expectedAdded) {
				t.Errorf("added: got %v, expected %v", added, tc.expectedAdded)
			}
			if !reflect.DeepEqual(modified, tc.expectedModified) {
				t.Errorf("modified: got %v, expected %v", modified, tc.expectedModified)
			}
			if !reflect.DeepEqual(deleted, tc.expectedDeleted) {
				t.Errorf("deleted: got %v, expected %v", deleted, tc.expectedDeleted)
			}
		})
	}
}

func TestGroupDeletedFiles(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) >= 4 && args[0] == "ls-tree" {
			current := args[3]
			if strings.Contains(current, "deleted-dir") {
				return "", "", nil
			}
			return "some tree content", "", nil
		}
		return "", "", nil
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), nil, nil)

	deleted := []string{
		"deleted-dir/file1.go",
		"deleted-dir/file2.go",
		"parent/deleted-dir/file3.go",
		"other-dir/file4.go",
	}

	folders, individual := groupDeletedFiles(a, "dummy-dir", deleted)

	expectedFolders := []string{"deleted-dir", "parent/deleted-dir"}
	expectedIndividual := []string{"other-dir/file4.go"}

	if !reflect.DeepEqual(folders, expectedFolders) {
		t.Errorf("folders: got %v, expected %v", folders, expectedFolders)
	}
	if !reflect.DeepEqual(individual, expectedIndividual) {
		t.Errorf("individual: got %v, expected %v", individual, expectedIndividual)
	}
}
