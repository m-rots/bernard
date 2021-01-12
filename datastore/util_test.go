package datastore

import (
	"reflect"
	"testing"
)

func TestRootFolders(t *testing.T) {
	type test struct {
		name     string
		input    []Folder
		roots    []Folder
		nonRoots []Folder
	}

	var testCases = []test{
		{
			name: "mixed",
			input: []Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
				{ID: "D", Parent: "Z"},
			},
			roots: []Folder{
				{ID: "A", Parent: "Z"},
				{ID: "D", Parent: "Z"},
			},
			nonRoots: []Folder{
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
			},
		},
		{
			name: "single file",
			input: []Folder{
				{ID: "A", Parent: "Z"},
			},
			roots: []Folder{
				{ID: "A", Parent: "Z"},
			},
		},
		{
			name: "roots only",
			input: []Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "X"},
				{ID: "C", Parent: "Y"},
			},
			roots: []Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "X"},
				{ID: "C", Parent: "Y"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			roots, nonRoots := RootFolders(tc.input)

			if !reflect.DeepEqual(roots, tc.roots) {
				t.Log(roots)
				t.Log(tc.roots)
				t.Errorf("roots do not align")
			}

			if !reflect.DeepEqual(nonRoots, tc.nonRoots) {
				t.Log(nonRoots)
				t.Log(tc.nonRoots)
				t.Errorf("non-roots do not align")
			}
		})
	}
}

func TestHierarchicalOrder(t *testing.T) {
	type test struct {
		name   string
		input  []Folder
		output []Folder
	}

	var testCases = []test{
		{
			name: "roots only",
			input: []Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "Y"},
				{ID: "C", Parent: "X"},
			},
			output: []Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "Y"},
				{ID: "C", Parent: "X"},
			},
		},
		{
			name: "ordered",
			input: []Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
				{ID: "D", Parent: "C"},
				{ID: "E", Parent: "B"},
				{ID: "F", Parent: "E"},
			},
			output: []Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
				{ID: "E", Parent: "B"},
				{ID: "D", Parent: "C"},
				{ID: "F", Parent: "E"},
			},
		},
		{
			name: "totally wrong order",
			input: []Folder{
				{ID: "C", Parent: "B"},
				{ID: "B", Parent: "A"},
				{ID: "A", Parent: "Z"},
			},
			output: []Folder{
				{ID: "A", Parent: "Z"},
				{ID: "B", Parent: "A"},
				{ID: "C", Parent: "B"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ordered := OrderFoldersOnHierarchy(tc.input)

			if !reflect.DeepEqual(ordered, tc.output) {
				t.Log(ordered)
				t.Log(tc.output)
				t.Errorf("output does not match expected output")
			}
		})
	}
}
