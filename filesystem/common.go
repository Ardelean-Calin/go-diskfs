package filesystem

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// SkipDir is used as a return value from [WalkFunc] to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var SkipDir error = fs.SkipDir

// SkipAll is used as a return value from [WalkFunc] to indicate that
// all remaining files and directories are to be skipped. It is not returned
// as an error by any function.
var SkipAll error = fs.SkipAll

// GenericWalk is a generic implementation for filesystem walking that should
// work on any type implementing the FileSystem interface.
func GenericWalk(filesystem FileSystem, root string, fn WalkFunc) error {
	infos, err := readDirInfos(filesystem, root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		for _, i := range infos {
			filename := filepath.Join(root, i.Name())
			err = walk(filesystem, filename, i, fn)
			if err == SkipDir || err == SkipAll {
				return nil
			}
		}
	}
	return err
}

func walk(filesystem FileSystem, path string, info fs.FileInfo, walkFn WalkFunc) error {
	if !info.IsDir() {
		return walkFn(path, info, nil)
	}

	infos, err := readDirInfos(filesystem, path)
	err1 := walkFn(path, info, err)
	// If err != nil, walk can't walk into this directory.
	// err1 != nil means walkFn want walk to skip this directory or stop walking.
	// Therefore, if one of err and err1 isn't nil, walk will return.
	if err != nil || err1 != nil {
		// The caller's behavior is controlled by the return value, which is decided
		// by walkFn. walkFn may ignore err and return nil.
		// If walkFn returns SkipDir or SkipAll, it will be handled by the caller.
		// So walk should return whatever walkFn returns.
		return err1
	}

	for _, fileInfo := range infos {
		name := fileInfo.Name()
		if name == "." || name == ".." {
			continue
		}

		filename := filepath.Join(path, name)
		if err != nil {
			if err := walkFn(filename, fileInfo, err); err != nil && err != SkipDir {
				return err
			}
		} else {
			err = walk(filesystem, filename, fileInfo, walkFn)
			if err != nil {
				if !fileInfo.IsDir() || err != SkipDir {
					return err
				}
			}
		}
	}
	return nil
}

// readDirInfos reads the directory named by dirname and returns
// a sorted (by name) list of directory infos.
func readDirInfos(f FileSystem, dirname string) ([]fs.FileInfo, error) {
	dirs, err := f.ReadDir(dirname)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(dirs, func(a, b fs.FileInfo) int {
		if strings.ToLower(a.Name()) >= strings.ToLower(b.Name()) {
			return 1
		} else {
			return -1
		}
	})

	return dirs, nil

}
