package plan_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-plugin-starter-template/build/sync/plan"
)

func TestCopyDirectory(t *testing.T) {
	assert := assert.New(t)

	// Create a temporary directory to copy to.
	dir, err := ioutil.TempDir("", "test")
	assert.Nil(err)
	defer os.RemoveAll(dir)

	wd, err := os.Getwd()
	assert.Nil(err)

	srcDir := filepath.Join(wd, "testdata")
	err = plan.CopyDirectory(srcDir, dir)
	assert.Nil(err)

	compareDirectories(assert, dir, srcDir)
}

func TestOverwriteDirectoryAction(t *testing.T) {
	assert := assert.New(t)

	// Create a temporary directory to copy to.
	dir, err := ioutil.TempDir("", "test")
	assert.Nil(err)
	defer os.RemoveAll(dir)

	wd, err := os.Getwd()
	assert.Nil(err)

	setup := plan.Setup{
		Template: plan.RepoSetup{
			Git:  nil,
			Path: wd,
		},
		Plugin: plan.RepoSetup{
			Git:  nil,
			Path: dir,
		},
	}
	action := plan.OverwriteDirectoryAction{}
	action.Params.Create = true
	err = action.Run("testdata", setup)
	assert.Nil(err)

	destDir := filepath.Join(dir, "testdata")
	srcDir := filepath.Join(wd, "testdata")
	compareDirectories(assert, destDir, srcDir)
}

func compareDirectories(assert *assert.Assertions, pathA, pathB string) {
	aContents, err := ioutil.ReadDir(pathA)
	assert.Nil(err)
	bContents, err := ioutil.ReadDir(pathB)
	assert.Nil(err)
	assert.Len(aContents, len(bContents))

	// Check the directory contents are equal.
	for i, aFInfo := range aContents {
		bFInfo := bContents[i]
		assert.Equal(aFInfo.Name(), bFInfo.Name())
		assert.Equal(aFInfo.Size(), bFInfo.Size())
		assert.Equal(aFInfo.Mode(), bFInfo.Mode())
		assert.Equal(aFInfo.IsDir(), bFInfo.IsDir())
	}

}
