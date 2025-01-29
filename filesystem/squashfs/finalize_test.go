package squashfs_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/squashfs"
	"github.com/diskfs/go-diskfs/testhelper"
)

var (
	intImage = os.Getenv("TEST_IMAGE")
)

// full test - create some files, finalize, check the output
func TestFinalizeSquashfs(t *testing.T) {
	blocksize := int64(4096)
	t.Run("valid", func(t *testing.T) {
		f, err := os.CreateTemp("", "squashfs_finalize_test")
		fileName := f.Name()
		defer os.Remove(fileName)
		if err != nil {
			t.Fatalf("Failed to create tmpfile: %v", err)
		}

		b := file.New(f, false)
		fs, err := squashfs.Create(b, 0, 0, blocksize)
		if err != nil {
			t.Fatalf("Failed to squashfs.Create: %v", err)
		}
		for _, dir := range []string{"/", "/FOO", "/BAR", "/ABC"} {
			err = fs.Mkdir(dir)
			if err != nil {
				t.Fatalf("Failed to squashfs.Mkdir(%s): %v", dir, err)
			}
		}
		var sqsfile filesystem.File
		for _, filename := range []string{"/BAR/LARGEFILE", "/ABC/LARGEFILE"} {
			sqsfile, err = fs.OpenFile(filename, os.O_CREATE|os.O_RDWR)
			if err != nil {
				t.Fatalf("Failed to squashfs.OpenFile(%s): %v", filename, err)
			}
			// create some random data
			blen := 1024 * 1024
			for i := 0; i < 5; i++ {
				b := make([]byte, blen)
				_, err = rand.Read(b)
				if err != nil {
					t.Fatalf("%d: error getting random bytes for file %s: %v", i, filename, err)
				}
				if _, err = sqsfile.Write(b); err != nil {
					t.Fatalf("%d: error writing random bytes to tmpfile %s: %v", i, filename, err)
				}
			}
		}

		sqsfile, err = fs.OpenFile("README.MD", os.O_CREATE|os.O_RDWR)
		if err != nil {
			t.Fatalf("Failed to squashfs.OpenFile(%s): %v", "README.MD", err)
		}
		dataBytes := []byte("readme\n")
		if _, err = sqsfile.Write(dataBytes); err != nil {
			t.Fatalf("error writing %s to tmpfile %s: %v", string(dataBytes), "README.MD", err)
		}

		fooCount := 75
		for i := 0; i <= fooCount; i++ {
			filename := fmt.Sprintf("/FOO/FILENAME_%d", i)
			contents := []byte(fmt.Sprintf("filename_%d\n", i))
			sqsfile, err = fs.OpenFile(filename, os.O_CREATE|os.O_RDWR)
			if err != nil {
				t.Fatalf("Failed to squashfs.OpenFile(%s): %v", filename, err)
			}
			if _, err = sqsfile.Write(contents); err != nil {
				t.Fatalf("%d: error writing bytes to tmpfile %s: %v", i, filename, err)
			}
		}

		err = fs.Finalize(squashfs.FinalizeOptions{})
		if err != nil {
			t.Fatal("unexpected error fs.Finalize()", err)
		}
		// now need to check contents
		fi, err := f.Stat()
		if err != nil {
			t.Fatalf("error trying to Stat() squashfs file: %v", err)
		}
		// we made two 5MB files, so should be at least 10MB
		if fi.Size() < 10*1024*1024 {
			t.Fatalf("resultant file too small after finalizing %d", fi.Size())
		}

		// now check the contents
		fs, err = squashfs.Read(b, 0, 0, blocksize)
		if err != nil {
			t.Fatalf("error reading the tmpfile as squashfs: %v", err)
		}

		dirFi, err := fs.ReadDir("/")
		if err != nil {
			t.Errorf("error reading the root directory from squashfs: %v", err)
		}
		// we expect to have 3 entries: ABC BAR and FOO
		expected := map[string]bool{
			"ABC": false, "BAR": false, "FOO": false, "README.MD": false,
		}
		for _, e := range dirFi {
			delete(expected, e.Name())
		}
		if len(expected) > 0 {
			keys := make([]string, 0)
			for k := range expected {
				keys = append(keys, k)
			}
			t.Errorf("Some entries not found in root: %v", keys)
		}

		// get a few files I expect
		fileContents := map[string]string{
			"/README.MD":       "readme\n",
			"/FOO/FILENAME_50": "filename_50\n",
			"/FOO/FILENAME_2":  "filename_2\n",
		}

		for k, v := range fileContents {
			var (
				f    filesystem.File
				read int
			)

			f, err = fs.OpenFile(k, os.O_RDONLY)
			if err != nil {
				t.Errorf("error opening file %s: %v", k, err)
				continue
			}
			// check the contents
			b := make([]byte, 50)
			read, err = f.Read(b)
			if err != nil && err != io.EOF {
				t.Errorf("error reading from file %s: %v", k, err)
			}
			actual := string(b[:read])
			if actual != v {
				t.Errorf("Mismatched content, actual '%s' expected '%s'", actual, v)
			}
		}

		validateSquashfs(t, f)

		// close the file
		err = f.Close()
		if err != nil {
			t.Fatalf("could not close squashfs file: %v", err)
		}
	})
}

//nolint:thelper // this is not a helper function
func validateSquashfs(t *testing.T, f *os.File) {
	// only do this test if os.Getenv("TEST_IMAGE") contains a real image for integration testing
	if intImage == "" {
		return
	}
	output := new(bytes.Buffer)
	/* to check file contents
	unsquashfs -ll /file.sqs
	unsquashfs -s /file.sqs
	*/
	mpath := "/file.sqs"
	mounts := map[string]string{
		f.Name(): mpath,
	}
	err := testhelper.DockerRun(nil, output, false, true, mounts, intImage, "unsquashfs", "-ll", mpath)
	outString := output.String()
	if err != nil {
		t.Errorf("unexpected err: %v", err)
		t.Log(outString)
	}
	err = testhelper.DockerRun(nil, output, false, true, mounts, intImage, "unsquashfs", "-s", mpath)
	outString = output.String()
	if err != nil {
		t.Errorf("unexpected err: %v", err)
		t.Log(outString)
	}
}

func TestFinalizeSquashfsWithSymlinks(t *testing.T) {
	blocksize := int64(4096)
	t.Run("valid with symlinks", func(t *testing.T) {
		f, err := os.CreateTemp("", "squashfs_finalize_symlink_test")
		fileName := f.Name()
		defer os.Remove(fileName)
		if err != nil {
			t.Fatalf("Failed to create tmpfile: %v", err)
		}

		b := file.New(f, false)
		fs, err := squashfs.Create(b, 0, 0, blocksize)
		if err != nil {
			t.Fatalf("Failed to squashfs.Create: %v", err)
		}

		// Create directories and files that will be targets for symlinks
		for _, dir := range []string{"/", "/target", "/links"} {
			err = fs.Mkdir(dir)
			if err != nil {
				t.Fatalf("Failed to squashfs.Mkdir(%s): %v", dir, err)
			}
		}

		// Create a target file
		targetFile, err := fs.OpenFile("/target/file.txt", os.O_CREATE|os.O_RDWR)
		if err != nil {
			t.Fatalf("Failed to create target file: %v", err)
		}
		content := []byte("target file content\n")
		if _, err = targetFile.Write(content); err != nil {
			t.Fatalf("Failed to write to target file: %v", err)
		}

		// Create symlinks
		symlinks := map[string]string{
			"/links/relative":    "../target/file.txt",
			"/links/absolute":    "/target/file.txt",
			"/links/dir":         "/target",
			"/links/nonexistent": "/does/not/exist",
		}

		for linkPath, target := range symlinks {
			err = fs.Symlink(target, linkPath)
			if err != nil {
				t.Fatalf("Failed to create symlink %s -> %s: %v", linkPath, target, err)
			}
		}

		err = fs.Finalize(squashfs.FinalizeOptions{})
		if err != nil {
			t.Fatal("unexpected error fs.Finalize()", err)
		}

		// Verify the filesystem by reading it back
		fs, err = squashfs.Read(b, 0, 0, blocksize)
		if err != nil {
			t.Fatalf("Failed to read back filesystem: %v", err)
		}

		// Check symlinks
		for linkPath, expectedTarget := range symlinks {
			fi, err := fs.OpenFile(linkPath, os.O_RDONLY)
			if err != nil {
				t.Errorf("Failed to open symlink %s: %v", linkPath, err)
				continue
			}
			stat, ok := fi.(os.FileInfo)
			if !ok {
				t.Errorf("Could not convert OpenFile() to FileInfo for %s", linkPath)
				continue
			}
			fileStat, ok := stat.(os.FileInfo)
			if !ok {
				t.Errorf("Could not convert to FileInfo for %s", linkPath)
				continue
			}
			symStat, ok := fileStat.Sys().(squashfs.FileStat)
			if !ok {
				t.Errorf("Could not convert Sys() to FileStat for %s", linkPath)
				continue
			}
			target, err := symStat.Readlink()
			if err != nil {
				t.Errorf("Failed to read symlink %s: %v", linkPath, err)
				continue
			}

			if target != expectedTarget {
				t.Errorf("Symlink %s points to %s, expected %s", linkPath, target, expectedTarget)
			}
		}

		// Try reading through a symlink
		file, err := fs.OpenFile("/links/relative", os.O_RDONLY)
		if err != nil {
			t.Fatalf("Failed to open file through symlink: %v", err)
		}

		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("Failed to read through symlink: %v", err)
		}

		if string(data) != string(content) {
			t.Errorf("Content through symlink doesn't match: got %q, want %q", string(data), string(content))
		}

		err = f.Close()
		if err != nil {
			t.Fatalf("could not close squashfs file: %v", err)
		}
	})
}
