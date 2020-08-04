package datastore

func RootFolders(folders []Folder) (roots []Folder, nonRoots []Folder) {
	IDtoParent := make(map[string]string)
	IDtoFolder := make(map[string]Folder)

	for _, folder := range folders {
		IDtoParent[folder.ID] = folder.Parent
		IDtoFolder[folder.ID] = folder
	}

	for _, f := range folders {
		if _, ok := IDtoParent[f.Parent]; ok {
			nonRoots = append(nonRoots, IDtoFolder[f.ID])
		} else {
			roots = append(roots, IDtoFolder[f.ID])
		}
	}

	return roots, nonRoots
}

func OrderFoldersOnHierarchy(nonRoots []Folder) (ordered []Folder) {
	for {
		if len(nonRoots) == 0 {
			break
		}

		roots, newNonRoots := RootFolders(nonRoots)
		nonRoots = newNonRoots

		ordered = append(ordered, roots...)
	}

	return ordered
}
